# CIR-009: Sentinels keep what they learn, and answer to names

## Intent

Let a Sentinel come back from a restart still knowing the failover, and let the
operator seed a master from what the Sentinels remember rather than from pod
order.

Carries out [ADR-001](../adr/ADR-001-sentinel-owns-leader-election.md), which
decides that Sentinel owns leader election and the operator only steps in when
Sentinel cannot, and [ADR-002](../adr/ADR-002-instances-are-addressed-by-name.md),
which decides that an instance written down somewhere is written down by name.

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
- WHEN it needs replacing
- THEN the operator replaces it, rather than the StatefulSet controller, so that
  the restart happens when the operator is able to point the replacement at a
  master

- GIVEN a failover with no master and Sentinels that remember one
- WHEN the operator seeds a master
- THEN it promotes the instance the Sentinels remember, and falls back to pod
  order only when they remember nothing

## Constraints

- **Only a StatefulSet gives a pod a name in DNS.** A Deployment's pods can be
  reached at an address and nothing else, so no addressing decision can be held
  for Sentinels while they run under one.
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
at an address, and an address a stranger can answer is what ADR-002 exists to
remove. It cannot be removed from a path that only some failovers take.

It also carried a second code path for as long as both were possible: a branch in
the ensurer, a branch in the pod lookup, a generator for each workload, and a
teardown that had to work out which of the two was unwanted. One kind of workload
removes all of it and leaves one migration.

The cost is a window on upgrade with no Sentinel able to elect. Measured on a two
node failover with 20000 keys, upgrading from `v4.7.1`: 30 seconds, all of it the
operator waiting for its next reconcile to point the new Sentinels at a master.
That figure assumes the rollout waits for a promotable replica before replacing
any Redis, which
[CIR-008](CIR-008-hold-the-master-until-a-failover-could-succeed.md) decides.
Without it the same upgrade measured 60 seconds, because the replica the Sentinels
needed to find was replaced while they were already blind.
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
Until then the operator's own writes keep the exposure ADR-002 removed everywhere
else, bounded by a pod list read in the same pass rather than by anything
remembered.

## Date

2026-09-30
