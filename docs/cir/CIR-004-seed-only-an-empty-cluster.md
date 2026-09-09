# CIR-004: Seed a master only where every node is empty

## Intent

Close the gap between [ADR-001](../adr/ADR-001-sentinel-owns-leader-election.md)
and the code. That decision says the operator seeds an initial master and does
not otherwise choose, and that it must not choose among nodes that may hold
data. Two paths in `CheckAndHeal` still chose without establishing that, so the
rule held in the decision record and not in the operator.

## Behavior

- GIVEN a `RedisFailover` with no master and no Sentinel quorum
- AND every running Redis holds no keys
- WHEN the operator reconciles
- THEN it seeds a master as before

- GIVEN a `RedisFailover` with no master and no Sentinel quorum
- AND at least one running Redis holds keys
- WHEN the operator reconciles
- THEN no node is promoted, the `MasterUnknown` condition records why, and the
  reconcile returns an error

- GIVEN a `RedisFailover` where every node reports localhost as its master
- AND at least one of them holds keys, which is a restart rather than a cold start
- WHEN the operator reconciles
- THEN no node is promoted

- GIVEN a node that cannot be reached to be asked whether it holds keys
- WHEN the operator reconciles
- THEN it is treated as though it holds data and nothing is promoted

- GIVEN a `RedisFailover` with exactly one Redis replica
- WHEN the operator reconciles with no master
- THEN it still sets that node as master. Negative: this path is unchanged,
  because a single candidate is not a choice among nodes

- GIVEN a `RedisFailover` with a master, or with Sentinel quorum intact
- THEN nothing about this change is reached. Negative: normal failover is
  untouched

## Constraints

- ADR-001 rules out ranking nodes by replication offset, so "which node is
  safest to promote" is not a question this may answer. The only safe case is
  the one where the question does not arise.
- The signal has to distinguish a cold start from a restart. Asking who a node
  replicates from cannot: a restarted pod reloads `slaveof 127.0.0.1` from its
  generated config while keeping its dataset.
- `MasterUnknown` and `reportMasterUnknown` already existed for holding off with
  a visible reason, so stopping needed no new mechanism.

## Decisions

- **The keyspace section of `INFO`, not `DBSIZE`.** `DBSIZE` answers for the
  database the client selected, and a node holding keys in any other database
  would read as empty. The keyspace section lists a line per database that holds
  keys and comes back empty when there are none.

- **A node that cannot be reached counts as holding data.** The two failure
  modes are not symmetric: treating an unreachable node as empty lets the
  operator promote over data it could not see, while treating it as holding data
  costs an availability stop that a person can resolve. ADR-001 already accepts
  that trade for the inspection path.

- **Both selecting paths are gated, not just the no-quorum one.** The
  first-boot path is guarded by `CheckIfMasterLocalhost`, which ADR-001 and
  CIR-003 both record as unable to tell a cold start from a restart. Leaving it
  ungated would have kept the same hole behind a different branch.

- **The single-replica path is left alone.** With one node there is no choice to
  make, and gating it would stop a standalone failover from ever getting a
  master.

- **Rejected: making this a warning and promoting anyway.** It would keep the
  availability behavior and record that the operator might have just lost
  writes, which is the outcome ADR-001 exists to prevent.

- **Rejected: keeping the message template in `reportMasterUnknown`.** It
  hardcoded "a Redis node could not be inspected, so one of them may still be
  the master", which is one cause of holding off and is now not the only one.
  Each caller supplies its own cause, so the condition names what actually
  happened.

## Date

2026-09-04
