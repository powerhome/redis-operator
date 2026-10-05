# CIR-009: Run the Sentinels as a set

## Intent

Give every Sentinel a name of its own in DNS.

Prepares for [ADR-004](../adr/ADR-004-the-operator-owns-how-instances-are-addressed.md),
which decides that how an instance is addressed is the operator's to settle. An
instance cannot be addressed by name until it has one, and under a Deployment a
Sentinel has none.

Using those names, rather than the addresses the operator still reads, is a
separate change.

## Behavior

- GIVEN any `RedisFailover`
- WHEN the operator reconciles it
- THEN its Sentinels run as a StatefulSet, each pod resolvable by name inside
  the namespace

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

**Every failover gets the set, not just some.** A name cannot replace an address
on a path only some failovers take, and leaving most of them reachable only at an
address would leave the thing a name exists to replace in place.

Running both workloads also meant carrying a second code path for as long as both
were possible: a branch in the ensurer, a branch in the pod lookup, a generator
for each, and a teardown that had to work out which of the two was unwanted. One
kind of workload removes all of it and leaves one migration.

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

**The operator still reaches Sentinels at an address.** `GetSentinelsIPs` returns
pod addresses whether the pods come from a set or a Deployment, so every
`SENTINEL MONITOR`, `RESET` and `CONFIG SET` goes to an address. What this change
buys is that a name now exists for every Sentinel, so a later change can use it.
Until then every such write is bounded by a pod list read in the same pass,
rather than by an address remembered from an earlier one.

## Date

2026-09-30
