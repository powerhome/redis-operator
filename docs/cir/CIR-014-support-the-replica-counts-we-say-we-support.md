# CIR-014: Support the replica counts we say we support

## Intent

Decide once which replica counts this operator supports for Redis, Sentinel and
HAProxy, and have every default and every generated resource follow that one
decision.

Nothing decided it before. Each mechanism answered the question in its own
corner and the answers disagreed. Two rolling update gates carve the small
counts out so an update does not wedge, and each arrived inside a fix for
something else: `hasReplicaToSpare` skips the failover gate at one Redis, and
`CheckSentinelsCanSpareOne` asks a weaker question when `replicas <= quorum`.
The Sentinel PodDisruptionBudget, sized from the Sentinel quorum, assumes those
counts do not exist. `spec.haproxy` has no default count at all, so an empty
block asks for a proxy and runs none.

## Behavior

- GIVEN a spec that carries `spec.haproxy` and names no replica count
- WHEN the operator validates it
- THEN the count is two, and a drain of one proxy leaves one serving

- GIVEN a spec that names a HAProxy replica count
- WHEN the operator validates it
- THEN that count stands

- GIVEN a spec with no `spec.haproxy` at all
- WHEN the operator reconciles it
- THEN no proxy resource exists, as before

- GIVEN a failover of three or more Sentinels
- WHEN the Sentinel disruption budget is generated
- THEN it keeps the Sentinel quorum

- GIVEN a failover of one or two Sentinels
- WHEN the Sentinel disruption budget is generated
- THEN it keeps all but one, and a drain of a node holding one proceeds

- GIVEN a failover of one Redis
- WHEN the Redis disruption budget is generated
- THEN it keeps none, and a drain of its node proceeds

- GIVEN any replica count a spec can carry
- WHEN a node holding one of those pods is drained
- THEN the eviction API permits at least one eviction

- GIVEN a failover already running on counts below a minimum
- WHEN this operator replaces the one that created it
- THEN it keeps reconciling, and no count is refused

## Constraints

- `Validate()` runs on every reconcile, at `operator/redisfailover/handler.go`,
  and an error returned there ends the reconcile and marks the cluster in error.
  A minimum enforced there would abandon the failovers that run on those counts
  today, so a count is never refused.
- Sentinel's own minimum is published by Redis, under "Fundamental things to
  know about Sentinel before deploying" and "Example Sentinel deployments":
  <https://redis.io/docs/latest/operate/oss_and_stack/management/sentinel/>
- `RedisFailoverStatus.Conditions` is a history rather than a set of current
  conditions. `AddCondition` appends when the type changes and keeps the last
  ten, and two call sites read the last element as the current state. A warning
  cannot live there without breaking both.

## Decisions

- **A disruption budget gives way rather than keep a whole set.**
  Each budget asks for at most `replicas - 1`. A budget
  that permits no eviction does not protect a failover: it blocks a node drain
  for as long as the budget exists, and the drain waits rather than fails. The
  counts that reach that cap are the ones that cannot survive a loss whatever
  the budget says. One Redis has no replica to promote, and a failover below
  three Sentinels cannot elect a leader. Measured on a four-node kind cluster:
  at three Sentinels the budget permits one eviction and refuses the second, and
  the cap changes nothing at three or more.
- **HAProxy defaults to two, and two is its minimum.** Redis and Sentinel
  default their counts, so `spec.haproxy: {}` reads as "on, with defaults". It
  produced a Deployment of zero replicas, which is a service with a name and no
  endpoints. One proxy would have survived only a rolling update, where
  `maxSurge` adds a pod before removing one. A drain, an eviction or a lost node
  does not surge, and no disruption budget covers HAProxy, so the client path
  goes with the pod. Two is what every example in this repository configures.
- **No count is refused.** The constraint above decides this: refusing would
  take failovers out of management for a configuration the operator accepted
  when it created them.
- **Deferred: how a count below a minimum reaches the person who set it.** The
  options are a status field rewritten each reconcile, a metric, or a log line.
  They differ in who sees them and in whether they clear by themselves, and the
  status list cannot carry it for the reason in the constraints. The budget and
  the default above hold whatever that choice turns out to be.

- **Deferred: a disruption budget for HAProxy.** Nothing bounds how many
  proxies a drain takes, so it can take every one of them at once. Adding a
  budget creates a resource for failovers that have none today, which is a
  change of its own.

## Date

2026-11-04
