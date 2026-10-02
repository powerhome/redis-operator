# CIR-010: Deprecate the pod networking fields

## Intent

Announce that `hostNetwork` and `dnsPolicy` are leaving the API, while they go on
doing what they say.

Carries out the first of the two releases
[ADR-004](../adr/ADR-004-the-operator-owns-how-instances-are-addressed.md)
decides on. The second removes the fields.

Nothing changes about what the operator builds. A failover setting either field
gets the same pods as before.

## Behavior

- GIVEN a `RedisFailover` that sets any of `redis.hostNetwork`,
  `redis.dnsPolicy`, `sentinel.hostNetwork` or `sentinel.dnsPolicy`
- WHEN the operator reconciles it
- THEN it logs one warning naming every one of those fields that is set, and
  builds the same pods it always did

- GIVEN the same failover and a reconcile every few seconds
- THEN the warning is said once per failover per operator process

- GIVEN a `RedisFailover` that sets none of them
- THEN nothing is logged

## Alternatives considered

**A status condition rather than a log line.** A condition is the better channel
for something the owner of a resource must act on, and it survives an operator
restart. It was not chosen here because the fields still work: there is nothing
degraded to report, and the last condition is a printer column on the resource,
so occupying it with a deprecation notice would hide a real one.

**A warning every reconcile.** A reconcile happens every few seconds, so this
reduces to filling the log with a notice nobody can act on faster than once.
Once per failover per process means a reader sees it on each operator rollout,
which is often enough to be noticed and rare enough to stay readable.

**Refusing the fields in validation.** This is the removal, arriving early and
without warning. A failover that sets them is working today, so refusing it on
upgrade breaks a running install to make a point the log line makes safely.

**Fixing the `hostNetwork` resolver trap at the same time.** A `hostNetwork` pod
with no explicit `dnsPolicy` gets the node's resolver, which cannot resolve
cluster names. Defaulting those pods to `ClusterFirstWithHostNet` would repair
it. It was left alone: the fields are going away, and changing how they behave on
the way out gives anyone still setting them a second behaviour change to absorb
rather than one deprecation to act on.
