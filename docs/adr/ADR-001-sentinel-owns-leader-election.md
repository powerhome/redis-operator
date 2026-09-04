# ADR-001: Sentinel owns leader election, the operator only seeds and records

## Status

Accepted

## Context

Two components can decide which Redis node is the master. Sentinel elects one by
consensus. The operator also picks one, in `SetOldestAsMaster`, reached from
three places in `CheckAndHeal` when it finds no master.

Nothing wrote down which of them is responsible, and the code disagrees with
itself about it. The branch that runs *with* Sentinel quorum checks
`CheckIfMasterLocalhost` before selecting, to confirm the cluster has never had
a master. The branch that runs *without* quorum selects unconditionally. The
path with consensus behind it is the cautious one; the path without consensus is
not.

Three facts shaped the decision.

**Sentinel cannot bootstrap itself.** Every Redis pod starts `slaveof 127.0.0.1`
and every Sentinel starts `sentinel monitor mymaster 127.0.0.1`, both from the
generated configuration. On a cold start there is no master anywhere, and each
Sentinel is watching its own localhost where nothing is listening. Sentinel
promotes a replica of a master it knows about, and discovers replicas from that
master's `INFO`, so with no reachable master it has no replica set and nothing to
promote. Something outside Sentinel has to create the first master.

**A cold start is not distinguishable from a restart by asking who the master
is.** `CheckIfMasterLocalhost` reports true when every reachable node names
localhost as its master. That is the cold-start signature, but it is also what a
full restart produces: pods reload the same generated `redis.conf`, which says
`slaveof 127.0.0.1`, while keeping whatever they had persisted. RDB persistence
is on by default (`save 900 1`, `save 300 10`) and `spec.redis.storage` can back
it with a volume. So a cluster holding data with divergent replication offsets
looks exactly like an empty one, which is the case where choosing wrongly costs
the most.

**Selection was by pod age.** `SetOldestAsMaster` sorts on `CreationTimestamp`,
so the oldest pod won regardless of how far behind its data was.

## Decision

Sentinel elects. The operator seeds an initial master and records what Sentinel
decides, and it does not otherwise choose.

Where the operator does have to act, because Sentinel cannot, it acts under three
rules:

1. **It acts only on a topology it has fully established.** Every running Redis
   node must answer before the operator concludes there is no master. A node it
   could not reach may be the master, and may hold the newest data.
2. **It does not choose among nodes that may hold data.** Seeding an empty
   cluster is safe because there is nothing to lose and every candidate is
   equivalent. Choosing between nodes that have taken writes is not, and the
   operator has no sound basis for it: see the rejected alternative below.
3. **It fails closed and says so.** When it cannot establish the topology it
   changes nothing and reports a condition on the resource naming the cause,
   rather than acting on a partial picture.

Rule 3 is what makes the operator's role narrow rather than absent: it never
guesses, but it is still the thing that gets an empty cluster to a first master.

## Consequences

**A running master is no longer replaced because the operator could not reach
it.** This was the path to divergent writes: an unreachable master counted as
absent, and recovery promoted over it while it was still serving.

**Recovery can stop and stay stopped.** A pod that stays `Running` while its
Redis never answers, a corrupt dataset behind a passing liveness probe, now
leaves the failover without a master where the operator would previously have
promoted among the reachable nodes and run degraded. That is an availability
regression, it does not resolve itself, and it is the deliberate price of not
guessing. The `MasterUnknown` condition exists so it is visible rather than
silent, and it surfaces in `kubectl get redisfailover` through the existing
printer column.

**Selection by replication position was investigated and rejected.** An earlier
draft of this decision made rule 2 "select the node with the highest replication
offset", on the grounds that this is what Sentinel does. Measuring the fields
against `redis:8.10.1` showed the evidence does not support it:

- **Replication IDs do not survive a restart.** Two nodes replicating from the
  same master shared `master_replid` while running, and each generated a fresh
  one on restart. A rule of "compare only when the IDs agree" would refuse in
  exactly the case it exists for, a whole cluster coming back after an outage.
- **`master_replid2` records ancestry, not safety.** Both restarted nodes did
  carry the original ID there, so shared history is recoverable. But two nodes
  promoted from the same master that each then accepted writes would also share
  it while holding conflicting data. Matching ancestry does not establish that
  the histories have not diverged.
- **The offset is not a measure of how much data a node holds.** A master with
  no replica attached reports `master_repl_offset:0` after writes, because the
  replication backlog is not created until one connects.

Recovering divergence from `INFO` after the fact means reconstructing
replication reasoning that Sentinel gets for free by deciding at the time of
failover, with consensus, among candidates it knows share a history. Putting a
reimplementation of that on the recovery path, where being wrong loses writes
silently, buys less than it costs.

So the operator does not rank nodes at all. Where a cluster has data and no
master can be established, rule 3 applies and a person decides.

**Pod age survives only where it cannot matter.** `SetOldestAsMaster` still
sorts on `CreationTimestamp`, and it is reached only where every candidate is
empty and equivalent, so the ordering decides nothing. Its name is misleading
about how little the ordering means; renaming it is worth doing.

**Distinguishing a cold start from a restart is load-bearing, and the keyspace
answers it.** Rule 2 says the operator may seed an empty cluster but must not
choose among nodes holding data, which requires telling those two apart.
`CheckIfMasterLocalhost` cannot: a restarted pod reloads `slaveof 127.0.0.1`
while keeping its dataset. `CheckIfAllRedisHoldNoData` asks each node for the
keyspace section of `INFO` instead, and every path that selects a master is
behind it. A node that holds keys, or that cannot be reached to be asked, makes
it false, so the operator reports `MasterUnknown` and changes nothing.

**`replica-priority` is still ignored.** Bootstrapping sets `replica-priority 0`
to keep a node from being promoted, and Sentinel honours it. Seeding an empty
cluster ignores it. That matters less now that the operator only seeds, but it
is still a stated intent the operator does not respect.

## Date

2026-09-02
