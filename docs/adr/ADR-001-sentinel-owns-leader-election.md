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
2. **It selects on the same evidence Sentinel would**, replication position,
   never on pod age. This removes the need to tell a cold start from a restart:
   on a genuinely empty cluster every offset is zero and any tie-break is
   correct, and on a restart the offsets order themselves.

   Replication position means `master_replid` and `master_repl_offset` together,
   not the offset alone. Offsets are only comparable between nodes that share a
   replication history. Sentinel never has to think about this because it ranks
   replicas of one known master, so the replid is the same across every
   candidate it considers. The operator arriving after a partition has no such
   guarantee: if two nodes accepted writes as masters their replids differ and
   their offsets are independent counters, so the larger number does not hold
   more of the writes anyone cares about. Where the replids agree the highest
   offset wins. Where they diverge the histories have forked, no automatic
   choice is safe, and rule 3 applies.
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

**Selecting on replication position needs something that does not exist yet.**
The Redis client exposes `IsMaster`, `GetSlaveOf` and `SlaveIsReady`, but nothing
reads `master_replid` or `master_repl_offset`. Until that is added, selection
remains by pod age and rule 2 is unmet. Rules 1 and 3 are implemented; rule 2 is
not.

Reading the offset alone would be worse than it looks. It would produce a
selector that appears principled, ranks nodes by a number, and still discards
writes whenever the histories have diverged, which is the case that motivates
this decision in the first place. The replid is what makes the comparison mean
anything, and its absence is what turns a forked history into a refusal rather
than a guess.

**`CheckIfMasterLocalhost` loses its role as a safety check.** It was the guard
distinguishing a cold start from a live cluster, and it cannot make that
distinction. Under rule 2 nothing needs it to.

**`replica-priority` is still ignored.** Bootstrapping sets `replica-priority 0`
to keep a node from being promoted, and Sentinel honours it. An operator that
selects on offset alone would promote a node its configuration says must never be
a master. Whether the operator should honour it too is unresolved.

## Date

2026-09-02
