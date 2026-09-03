# CIR-003: Fail closed when the master cannot be verified

## Intent

Stop the operator replacing a Redis master it could not see, and stop it
reporting success when it left two.

Implements rules 1 and 3 of [ADR-001](../adr/ADR-001-sentinel-owns-leader-election.md),
which decides that the operator acts only on a topology it has fully
established, and otherwise changes nothing and says so.

Rule 2, that the operator does not choose among nodes which may hold data, is
partly met: it no longer promotes over a node it could not inspect. It still
selects by pod age when it does act, and still cannot tell an empty cluster from
a restarted one, both recorded in the ADR as open.

Two faults, both reported in
[issue 100](https://github.com/powerhome/redis-operator/issues/100) and both
verified against the tree before starting.

`GetNumberMasters` skipped past any node it could not query and counted only the
ones that answered as a master, so a master that was running and writable but
momentarily unreachable was reported absent. That count reaching zero is what
drives recovery, and recovery promotes a node.

`SetOldestAsMaster` logged a failure to demote a node and carried on to return
nil. Promoting one node while failing to demote another leaves two masters and a
success, until a later reconcile stops with "more than one master, fix manually"
and divergent writes are already possible.

## Behavior

- GIVEN every running Redis node answers and none of them is the master
- WHEN the operator reconciles
- THEN it proceeds to recovery as before

- GIVEN a running Redis node that cannot be inspected, and no master found among
  the ones that answered
- WHEN the operator reconciles
- THEN it changes nothing, and records a `MasterUnknown` condition naming the
  cause

- GIVEN a reachable master and an unreachable peer
- WHEN the operator reconciles
- THEN the peer is irrelevant and nothing is held back: the master settles it

- GIVEN a node that refuses the configured credential
- WHEN the operator reconciles
- THEN it takes the credential path from [CIR-002](CIR-002-apply-a-password-change.md)
  and restarts the pods, which is reported ahead of an unreachable peer when both
  happened in one pass

- GIVEN a demotion that fails during recovery
- WHEN the remaining nodes have been demoted
- THEN every failure is returned, and the pod that could not be demoted is not
  labelled `slave`

- GIVEN a pod that is `Pending`, terminal, or being deleted
- WHEN the operator reconciles
- THEN it is filtered out before any of this and SHOULD NOT block anything

- GIVEN a `RedisFailover` with no fault
- WHEN the operator reconciles
- THEN its behaviour SHOULD NOT change

## Constraints

- **No new client surface.** Selecting on replication offset needs an accessor
  the Redis client does not have. Adding it belongs with rule 2, so this change
  works with what `IsMaster` already reports.
- **The hold must be visible.** Refusing to act can last indefinitely, so it
  cannot be reported only as a reconcile error every thirty seconds. It uses the
  existing condition mechanism, whose last message is already a printer column.
- **Do not write status on every pass.** The state can persist, so the condition
  is recorded once rather than rewritten each reconcile.

## Decisions

- **Rejected: bounding the wait.** After some number of passes blocked only by an
  unreachable node, proceed on what is known. This restores the availability the
  change costs, and was rejected because it reintroduces the fault at a delay:
  the node that never answers is exactly the one that might be the master. Left
  as the obvious softening if the trade is judged too sharp in practice.
- **Rejected: treating any failed inspection as fatal.** Returning an error
  whenever any node cannot be reached, regardless of the count, would stop
  healing a cluster that has a perfectly good reachable master. The error is
  returned only when nothing answered as a master, since a reachable master
  settles the question.
- **Rejected: keeping only the first demotion failure.** The first version used
  an `if demotionErr == nil` guard. Each failure is a node still acting as a
  master, so naming one understates how far the failover is from having a single
  master. `errors.Join` collects them, and removes a nil-check wrapped around an
  assignment to the thing being checked, which read as a no-op.
- **Rejected: `CheckIfMasterLocalhost` as a safety check.** Proposed as the way
  to tell a cold start from a live cluster, so the operator could seed freely in
  the first case. It cannot make that distinction: a full restart reloads
  `slaveof 127.0.0.1` from the generated config while keeping persisted data, so
  a cluster with data looks identical to an empty one.
- **Rejected: selecting by replication offset.** Pursued as far as a working
  accessor before measurement against `redis:8.10.1` showed the fields do not
  support it. Replication IDs are regenerated on restart, `master_replid2`
  records ancestry rather than absence of divergence, and a master with no
  replica attached reports offset zero after writes. ADR-001 carries the detail;
  the outcome is that the operator does not rank nodes at all.
- **Accepted: three existing unit tests were changed.** They asserted the
  behaviour removed here, including one asserting that a failed demotion reports
  success. They encoded the defect as the contract rather than describing an
  intended one. Changing tests to match new code deserves scrutiny, so it is
  recorded here rather than left to be noticed in review.

## Verification

Both changes were mutation-tested rather than assumed. Removing the error
collection fails two tests, and the collection behaviour is asserted with two
distinct errors rather than inferred.

Exercised on a live cluster, which is the only place the behaviour can be seen,
by blocking every Redis with `CLIENT PAUSE` while the pods stayed `Running`:

- A healthy failover reconciled with no errors and no change to its conditions.
- With no node answering, the reconcile stopped with `i/o timeout`, promoted
  nothing, and recorded `MasterUnknown` naming the cause.
- When the pause expired the operator resumed, the failover returned to one
  master with a replica linked up, and the condition returned to `Ready`. The
  hold releases rather than wedging.

`DEBUG SLEEP` was the first attempt at blocking and does nothing: Redis 8
disables `DEBUG` unless `enable-debug-command` is set, so the run proved nothing
until `CLIENT PAUSE` replaced it.

No coverage in the automated integration suite. It cannot induce selective
inspection failures or partitions, and issue 100 asks for exactly that as part of
the larger fix.

## Date

2026-09-02
