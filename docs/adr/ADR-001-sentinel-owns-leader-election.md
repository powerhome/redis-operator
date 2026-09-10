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

Five restarts of a `RedisFailover` with two Redis and three Sentinels, plus one
run with a hand-configured Sentinel described further down, on a local `kind`
cluster, Redis and Sentinel 8.4.0, quorum 2, storage backed by a
persistent volume claim per pod. The operator was built before any of it and
chooses exactly as described above. Two replicas is what this fleet runs, which
is worth stating because losing one loses quorum.

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

**Restarting both, with data on disk.** The same as the second run, with 5000
keys written and an 88KB `dump.rdb` on each node's persistent volume first. The
operator took the no-quorum branch and chose:

```
21:12:41  insufficnet sentinel to reach Quorum - Unhealthy count: 3
21:12:41  Quorum not available for sentinel to choose master ... Operator to step-in
21:12:41  New master is rfr-redis-1 with ip 10.244.2.21
```

Both nodes returned holding all 5000 keys and the failover reformed around the
chosen master. So the path this decision forbids, selecting among nodes that
hold data, is the path that recovers a restarted cluster today, and with the
Sentinels wiped nothing else was in a position to.

Note what this does not show. Both nodes came back with identical datasets, so
no choice among them could have lost a write. It establishes that the path is
taken with data present, not that taking it loses data. That took two more runs.

**Restarting both, with the datasets divergent.** Run twice, differing only in
which node held writes the other lacked. The operator and the Sentinels were
stopped while the divergence was built, because Sentinel reverses an unwanted
promotion on sight, logging `+convert-to-slave`. Each node was then given its
own dataset, persisted to its own volume, and both Redis pods were restarted
together before the Sentinels and the operator were brought back.

| Newer writes on | Operator promoted | Result |
| --- | --- | --- |
| `rfr-redis-0` | `rfr-redis-0` | the writes survived |
| `rfr-redis-1` | `rfr-redis-0` | 2000 committed writes destroyed |

In the second, `rfr-redis-1` returned holding 9001 keys including 2000 that
existed nowhere else. The operator promoted `rfr-redis-0`, holding 7001 and none
of them, and `rfr-redis-1` was re-slaved and resynchronised down to the master's
dataset. Nothing recorded that anything had been discarded: no error, no
condition, and a failover reporting healthy.

**The selection is not by age in this case.** Both pods carry the same creation
timestamp to the second after a simultaneous restart:

```
rfr-redis-0   2026-09-09T21:24:46Z
rfr-redis-1   2026-09-09T21:24:46Z
```

`SetOldestAsMaster` sorts on `CreationTimestamp.Before()`, which finds no
ordering between equal values, so the result is the order the pod list arrived
in. It promoted `rfr-redis-0` in both runs, whichever node held the data. So in
the case this decision is most concerned with, the choice is not a weak
heuristic over pod age. It is the lowest ordinal, deterministically, with no
relationship to the data at all.

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

That choice is what puts the operator in front of the decision at all. Where the
Sentinels keep their configuration, the first run shows them electing correctly
without help. Where they lose it, the operator selects the lowest ordinal on a
cluster holding data because nothing else can, and the last run shows that
losing committed writes. Making that configuration durable would leave the
choice with the component this decision says should make it, rather than
deciding how the operator should make it.

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

**Why discarding that configuration protects something.** Measured, and it
argues against the option above.

Sentinel does not only observe a topology, it enforces one. The runs above
caught it doing so: detaching a replica produced `+convert-to-slave` and a
`REPLICAOF` dragging the node back. Everything it enforces against is addressed
by pod IP, and Kubernetes reissues pod IPs.

So a Sentinel that survives a restart holding `known-replica mymaster
10.244.2.21 6379` may find that address now belongs to a Redis in another
namespace, serving a different `RedisFailover`. Seeing it report `role:master`
where a replica is expected, Sentinel would correct it, and that correction is
`REPLICAOF` pointed at a master in the first cluster. The second cluster's
master becomes a replica and resynchronises away its own dataset. The same
confusion on a monitored master address is worse again: two Sentinel quorums
issuing conflicting instructions to the same pods.

This was reproduced rather than reasoned about. Two standalone Redis instances
were placed in separate namespaces, neither authenticated, matching the fleet
default. One Sentinel was started in the first namespace with a hand-written
configuration of the shape one would persist: it monitored its own master, and
carried a single `known-replica` line naming the address of the Redis in the
second namespace, which is what a recycled pod IP produces.

```
21:40:34  +monitor master mymaster 10.244.3.23 6379 quorum 1
21:40:44  +convert-to-slave slave 10.244.2.26:6379 ... @ mymaster 10.244.3.23 6379
```

Ten seconds. The Redis in the second namespace was left as:

```
role:slave
master_host:10.244.3.23        a master in a different namespace
dbsize: 10                     it held 500
foreign:1 still present?  0    its own data, discarded
owner:1 now present?      1    it now serves the other cluster's data
```

All 500 of its keys were destroyed by the resynchronisation. From the first
cluster's side nothing was wrong: it gained a replica and said so,
`connected_slaves:1`. No error was raised at any layer, and no failover, quorum
negotiation or election was involved.

Note which line did it. The stale entry was a `known-replica`, not the monitored
master, so Sentinel does not need to be confused about its own master for this
to happen. One recycled replica address is enough, and a failover has more
replicas than masters.

Nothing in this operator would prevent it. There is no `resolve-hostnames`,
`announce-ip` or `announce-hostnames` anywhere, so every address it hands
Sentinel is a pod IP. Every `RedisFailover` serves Redis on 6379. The network
policy that would isolate namespaces is only created when `networkPolicyNsList`
is set. And per CIR-001, authentication that would reject a foreign connection
was unset on all 122 `RedisFailover` resources across four clusters.

Wiping the configuration on every start forecloses all of it. Sentinel restarts
knowing nothing, so the only topology it can act on is the one the operator
gives it, and the operator is what knows where a namespace ends. That is the
only thing standing between this operator and the run above, whatever was
intended when it was written, and it is the reason durability cannot be added on
its own. It would need an identity that survives
IP reuse: hostnames through `sentinel resolve-hostnames`, or a password per
failover, or a network policy that is not optional.

Set against that, what the wipe costs is the restart case, where the operator
selects the lowest ordinal and can lose committed writes.

**An identity that survives IP reuse already exists, unused.** A StatefulSet pod
has a stable name in DNS, `rfr-redis-0.rfr-redis.redis-test.svc.cluster.local`,
which encodes the namespace and the set it belongs to. Nothing in another
namespace can answer to it. The `rfr-redis` headless service that grants those
names is already created, and they already resolve. The operator addresses
everything by pod IP anyway.

Whether that helps depends on Sentinel re-resolving the name rather than
resolving once and keeping the address, which would leave it holding a
recyclable IP after all. It re-resolves. A Sentinel configured with
`resolve-hostnames yes` and monitoring a pod by name, whose pod was then deleted
and recreated on a new address:

```
11:53:30  +sdown / +odown  master mymaster dnsredis-0.peers.dns-test...
11:53:41  -sdown / -odown  master mymaster dnsredis-0.peers.dns-test...
```

It lost the instance, and eleven seconds later had followed the name to the new
address with no reconfiguration, `10.244.2.30` to `10.244.2.31`. It also stores
and reports the master as the hostname rather than a resolved address. So a
stale entry naming a pod resolves to the pod it means or fails to resolve, and
failing leaves Sentinel inert rather than acting on a stranger.

Three things would be needed, and the awkward one was measured rather than
assumed:

- `sentinel resolve-hostnames yes`, which requires Redis 6.2 or newer. The
  default image is `redis:7.2.4-alpine`.
- `replica-announce-ip` set to each pod's own name, because Sentinel discovers
  replicas from the master's `INFO replication`, which otherwise reports the raw
  IP the replica connected from.
- `publishNotReadyAddresses` on the governing headless service. Without it a pod
  has no DNS record until it is ready, and during a restart Redis is not ready
  for as long as it takes to load its dataset, which is exactly the window
  Sentinel would need to reconnect in.

That last one decides whether the idea works at all, so it was run. Deleting a
pod and resolving its name from another pod, first as the operator configures
the service today and then with the field set:

```
publishNotReadyAddresses unset          publishNotReadyAddresses: true
t+12s  redisReady=false  UNRESOLVED     t+12s  redisReady=false  10.244.2.29
t+36s  redisReady=false  UNRESOLVED     t+36s  redisReady=false  10.244.2.29
t+42s  redisReady=true   10.244.2.28    t+84s  redisReady=false  10.244.2.29
```

Unset, the name is absent for the whole not-ready window and appears only when
readiness returns. Set, it resolves throughout and tracks the new address within
six seconds of the pod being recreated.

It has to be the governing service. A second headless service selecting the same
pods gets `hostname` empty in its endpoints, because a StatefulSet fixes each
pod's subdomain to the service named in `serviceName`. So this is a change to
the operator rather than something that can be added beside it.

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
