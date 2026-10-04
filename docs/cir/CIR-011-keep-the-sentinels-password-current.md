# CIR-011: Keep each Sentinel's password for the master current

## Intent

Keep every Sentinel able to reach the master it monitors, so that it goes on
reading the master's replica list and holds something it could promote.

Addressing instances by name, decided in
[ADR-002](../adr/ADR-002-instances-are-addressed-by-name.md) and carried out in
[CIR-006](CIR-006-address-redis-by-dns-name.md), removed the only thing that was
keeping that password current.

## Behavior

- GIVEN a `RedisFailover` whose password changes
- WHEN the operator reconciles after the Redis pods have been replaced
- THEN every Sentinel is given the current password, and reports the master up

- GIVEN a `RedisFailover` with no password
- WHEN the operator reconciles
- THEN no password is sent, and a Sentinel holding a previous one is left alone,
  because a Sentinel offering a password to a Redis that wants none is still
  served

- GIVEN a Sentinel that refuses the password
- WHEN the operator reconciles
- THEN the failure names that Sentinel, and every other Sentinel is still
  reached

- GIVEN a Sentinel already monitoring the right master with the right password
- WHEN the operator reconciles
- THEN it is sent the same password again, which changes nothing

## Constraints

- **`SENTINEL REMOVE` discards the password along with the master.** Every
  repointing passes through that loss, which is why
  `MonitorRedisWithPort` already restores it. What it does not cover is a
  Sentinel that was never repointed.
- **A Sentinel rewrites its own configuration file.** `SENTINEL RESET` therefore
  re-reads the stale password rather than clearing it, so resetting a Sentinel
  does not recover one.
- **Nothing the operator reads reports this.** A Sentinel in this state answers
  `SENTINEL get-master-addr-by-name` with the right master, so the monitored
  address matches and no existing check fires. Its own readiness probe passes
  for the same reason.

## Decisions

**The password is reapplied on every pass, in the loop that already reapplies
the custom config.** Measured against Redis 7: a Sentinel whose master restarts
with a different password reports
`flags: s_down,o_down,master,disconnected` and stops refreshing `INFO`, so
`num-slaves` stays at whatever it last read and `SENTINEL replicas` empties.
Sending `SENTINEL SET mymaster auth-pass` restored it within three seconds with
no restart, which is what makes reapplying it sufficient.

**Why the address comparison used to hide this.** `CheckSentinelMonitor` was
given the master's address, and replacing a pod changes it, so the comparison
failed after every password change and `NewSentinelMonitor` repointed the
Sentinel and restored the password as a side effect. Giving it the master's name
instead, which is the point of CIR-006, leaves the comparison matching and the
password untouched. The repair was never deliberate.

**Rejected: detecting the state and repointing.** Reading each Sentinel's flags
for `disconnected` and calling `NewSentinelMonitor` when it appears would
restore the password through the existing path. It needs a new check, a new
reading of Sentinel's own vocabulary, and it repairs after the fact rather than
keeping the state current. One idempotent command per Sentinel per reconcile
costs less and has no state to get wrong.

**Rejected: clearing the password when a `RedisFailover` has none.** A Sentinel
holding a password for a Redis that requires none was measured to stay
connected and keep reading the replica list, so there is nothing to repair.
`SENTINEL SET mymaster auth-pass ""` is accepted, and could be sent, but it
would be machinery for a state that does not break.

**Rejected: restarting the Sentinel pods.** Deleting them does recover a
failover, because a fresh pod reads the generated configuration, monitors
`127.0.0.1`, fails the monitor comparison and is repointed with the current
password. It is also an outage of the quorum to fix something one command
settles.

## Date

2026-10-03
