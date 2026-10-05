# CIR-009: Sentinels keep what they learn

## Intent

Let a Sentinel come back from a restart still knowing the failover.

Carries out [ADR-001](../adr/ADR-001-sentinel-owns-leader-election.md), which
decides that Sentinel owns leader election and the operator only steps in when
Sentinel cannot.

Seeding a master from what the Sentinels remember is a separate change.

## Behavior

- GIVEN a `RedisFailover` with `sentinel.storage.persistentVolumeClaim` set
- WHEN the operator reconciles it
- THEN each Sentinel gets a claim of its own, and what it writes to its
  configuration survives the pod

- GIVEN a `RedisFailover` with no sentinel storage
- WHEN the operator reconciles it
- THEN the Sentinels still run as a set, and their configuration sits on scratch
  space as it always has

- GIVEN a failover whose Sentinels an earlier release ran as a Deployment
- WHEN this operator first reconciles it
- THEN the Deployment is removed and the set created, in that order, and the
  failover spends a window with no Sentinel able to elect

- GIVEN a set of Sentinels being created
- WHEN they start
- THEN they start together rather than in an ordinal chain, because they have no
  order between them and a readiness only the operator can produce would
  otherwise stall the rollout holding two of three

- GIVEN a Sentinel running an old pod template
- WHEN the Sentinels that would remain could still agree a failover
- THEN the operator replaces it, one Sentinel per pass, rather than the
  StatefulSet controller, so the restart happens when the operator is able to
  point the replacement at a master

- GIVEN a Sentinel running an old pod template and a quorum that could not
  survive losing it
- WHEN the operator reconciles
- THEN the pod is left alone and the operator says how many Sentinels report the
  master and how many must remain

- GIVEN a failover with no more Sentinels than its own quorum
- WHEN one of them runs an old pod template
- THEN it is replaced once every Sentinel reports the master, because no answer
  would permit it otherwise and the pod would never be replaced at all

## Constraints

- **Only a StatefulSet gives a pod a name in DNS.** A Deployment's pods can be
  reached at an address and nothing else. A pod's address outlives the pod and
  can be reissued to an unrelated pod, so nothing durable can be written down
  about a Sentinel while it runs under a Deployment.
- **A Sentinel monitoring `127.0.0.1` never reports ready**, because its
  readiness is that it monitors something else. Only the operator can change
  that, so nothing that waits on Sentinel readiness may sit between the operator
  and the Sentinel.
- **The operator needs one reconcile to configure Sentinels it has just
  created.** They do not exist when the pass that creates them runs, and the next
  pass is a resync away. This is what the migration window is made of.
- **Both workloads produce pods under the same labels.** Running them together is
  six Sentinels for a failover that asked for three, finding each other and
  agreeing a quorum among all of them, so the Deployment goes before the set is
  created rather than after.
- **The migration is idempotent in one direction only.** An operator that runs
  the Sentinels as a set removes a Deployment it finds; an older operator that
  finds a set leaves it running. Two operators reconciling one failover across an
  upgrade can therefore leave both, which is worth knowing when more than one
  replica of the operator is running.

## Decisions

**The Sentinels run as a set whether or not they have storage.** Making it
conditional on storage would leave most failovers with Sentinels reachable only
at an address, which is the thing a name exists to replace. It cannot be replaced
on a path that only some failovers take.

It also carried a second code path for as long as both were possible: a branch in
the ensurer, a branch in the pod lookup, a generator for each workload, and a
teardown that had to work out which of the two was unwanted. One kind of workload
removes all of it and leaves one migration.

The cost is a window on upgrade with no Sentinel able to elect. Measured on a two
node failover with 20000 keys, upgrading from `v4.7.1`: 30 seconds, all of it the
operator waiting for its next reconcile to point the new Sentinels at a master.
That figure assumes the rollout waits for a promotable replica before replacing
any Redis, which is a separate change. Without it the same upgrade measured 60
seconds, because the replica the Sentinels needed to find was replaced while they
were already blind.
Nothing a client sees depends on it, because HAProxy selects the master by asking
each Redis for `role:master` rather than by asking Sentinel: of 3548 writes
through it during the upgrade, none were refused during the migration, and the
only refusals were the two seconds of the master's own replacement.

**The remaining 30 seconds is left alone.** Closing it means the operator coming
back before its next resync, and the only lever is `ProcessingJobRetries`, which
is zero, so no handler error requeues at all today. Turning it on changes every
error path in the operator to retry with backoff, which is a larger decision than
this window justifies and wants its own measurement. The wait is also not created
here: it is how long the operator has always taken to configure Sentinels it has
just created, on a fresh install as much as an upgrade.

**Storage keeps what a Sentinel learned.** A claim is retained when its pod is
deleted, and the init container seeds the configuration only where there is
none, so a replacement rebinds its own volume and starts from the file the
previous pod wrote.

That makes a roll cheap: the replacement reports the master at startup and needs
nothing from the operator to become ready, where without storage it begins on
`127.0.0.1` and waits a reconcile, the window the quorum question above exists
to bound. Measured at 5 seconds to report the master with storage against 21
without.

It also decides what a failover has left when every Sentinel restarts at once.
With storage each one reads back the master and its peers, so they re-form a
quorum and can carry out a failover themselves, which is what ADR-001 asks of
them. Without storage they come back blank, and the only remaining account of
the topology is what the operator can re-derive, which `GetMasterIP` does by
asking each Redis for `role:master` and requires exactly one answer. A failover
where no Redis reports a master has nobody left who knows which node was one.

That is a second reason to run the set for every failover rather than only those
asking for storage.

**Turning storage off removes the claims.** Kubernetes keeps a claim when its
volume claim template is removed from a set, and nothing else would ever delete
it. The same seed guard that makes a replacement cheap then works against a
failover that is given storage again later: the file is not empty, so it is not
reseeded, and the Sentinel resumes the topology from whenever storage was last
declared. Measured on a three Sentinel failover whose master was failed over
while storage was off, each Sentinel came back monitoring the previous master's
address and stayed there for up to 25 seconds, until the operator's next pass
repointed it. Kubernetes reuses pod addresses, so the one a
Sentinel resumes may by then belong to an unrelated pod, and a Sentinel that
passes its own readiness check while monitoring one is worse than a Sentinel
that starts blank: the check only rejects `127.0.0.1`.

So the operator deletes the claims when a failover stops declaring Sentinel
storage. While a Redis reports `role:master` the operator re-derives the
topology and points a blank Sentinel at it within a reconcile, so a file that
disagrees with it costs more than no file at all. What a failover gives up by
turning storage off is the protection described above, which is its own to
decide. The Redis dataset is never re-derivable, which is why this is
deliberately not a general rule about claim templates being removed, and does
not live in the update path that Redis shares.
`storage.keepAfterDeletion` does not apply, because it governs what happens when
the `RedisFailover` is deleted and it lives inside the stanza whose removal
triggers this.

**The operator still reaches Sentinels at an address.** `GetSentinelsIPs` returns
pod addresses whether the pods come from a set or a Deployment, so every
`SENTINEL MONITOR`, `RESET` and `CONFIG SET` goes to an address. What this change
buys is that a name now exists for every Sentinel, so a later change can use it.
Until then every such write is bounded by a pod list read in the same pass,
rather than by an address remembered from an earlier one.

## Date

2026-09-30
