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

## Measured behaviour of Sentinel across a restart

Recorded 2026-09-09, after this decision was accepted. It qualifies one claim in
the Context above, so it is written here rather than left in a pull request.

Two restarts of a `RedisFailover` with two Redis and three Sentinels, on a local
`kind` cluster, Redis and Sentinel 8.4.0, empty dataset, quorum 2. The operator
was built before any of it and chooses exactly as described above.

**Restarting only the Redis pods, leaving the Sentinels running.** Sentinel
handled it, and was not allowed to finish:

```
17:58:27.772  +sdown master mymaster 10.244.3.10 6379
17:58:27.872  +odown master mymaster #quorum 2/2
17:58:27.867  +vote-for-leader 27f4b8b9... 1
17:58:27.873  Next failover delay: I will not start a failover before 17:58:48
```

It reached objective down with quorum, elected a leader, and scheduled its
failover. At `17:58:39`, nine seconds before that was due, the operator selected
a master by pod age and reconfigured all three Sentinels onto it, which appears
in their logs as `-monitor` then `+monitor` then `+reset-master`. Sentinel never
ran the election it had already won the right to run.

**Restarting the Redis and Sentinel pods together.** Sentinel did nothing:

```
20:34:51.636  +monitor master mymaster 127.0.0.1 6379 quorum 2
20:34:52.687  +sdown master mymaster 127.0.0.1 6379
              (fifty seconds, no further entries)
20:35:42.290  -monitor master mymaster 127.0.0.1 6379
```

It came back monitoring localhost, marked localhost down, and stopped. No
`+odown`, no epoch, no vote. The operator repointed it fifty seconds later,
taking the no-quorum branch: `insufficnet sentinel to reach Quorum - Unhealthy
count: 3`.

**What this qualifies.** The Context says that with no reachable master Sentinel
"has no replica set and nothing to promote". That is what the second run shows,
and it is not what the first shows. The difference is not a property of Sentinel.
It is where this operator keeps Sentinel's configuration: `sentinel-config-writable`
is an `emptyDir`, and the `sentinel-config-copy` init container overwrites it
from the ConfigMap on every pod start. Sentinel writes its learned topology to
that file continuously while running, logging `Sentinel new configuration saved
on disk`, and the operator destroys it on the next start.

So Sentinel cannot bootstrap itself, which remains true and is why the operator
seeds. But a cluster returning from a restart is not bootstrapping, and the
reason Sentinel cannot recover it is a choice recorded nowhere.

**A second finding, from the first run.** Where the Sentinels survive, the
operator preempts a working failover by seconds and selects by pod age, while
Sentinel was about to select with consensus among candidates it knew shared a
history. On an empty cluster that is harmless. On a populated one the operator
would be making exactly the choice this decision says it must not make, in a
case where the component that should make it was ready to.

**What this opens, and does not settle.** Giving Sentinel durable configuration
and an address that survives rescheduling would let it handle the restart case
on its own, which is what the first run demonstrates it can do. Whether that
holds when the addresses it remembers are stale pod IPs is not measured, and
neither is the effect on a genuine cold start. This is recorded as an option the
decision did not weigh, not as a change to it.

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
sorts on `CreationTimestamp`, which is now reached only when seeding, where every
candidate is empty and equivalent. Its name is misleading about how little the
ordering means; renaming it is worth doing.

**Distinguishing a cold start from a restart is now load-bearing, and nothing
does it yet.** Rule 2 says the operator may seed an empty cluster but must not
choose among nodes holding data, which requires telling those two apart.
`CheckIfMasterLocalhost` cannot: a restarted pod reloads `slaveof 127.0.0.1`
while keeping its dataset. Something like "no node holds any keys" would serve,
and is not implemented. Until it is, the operator still seeds on a signal that
cannot tell the difference, which is the largest remaining gap against this
decision.

**`replica-priority` is still ignored.** Bootstrapping sets `replica-priority 0`
to keep a node from being promoted, and Sentinel honours it. Seeding an empty
cluster ignores it. That matters less now that the operator only seeds, but it
is still a stated intent the operator does not respect.

## Date

2026-09-02
