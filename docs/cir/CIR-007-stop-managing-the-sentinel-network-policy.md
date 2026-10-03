# CIR-007: Stop managing the Sentinel NetworkPolicy

## Intent

Stop the operator owning network isolation, without breaking a `RedisFailover`
that still asks for it.

Carries out [ADR-003](../adr/ADR-003-network-isolation-is-not-the-operators.md),
which decides that confining these pods belongs to whoever runs the cluster, and
records the measurements behind that.

## Behavior

- GIVEN a failover whose Sentinels have a policy the operator wrote
- WHEN the operator reconciles
- THEN the policy is deleted

- GIVEN a failover that never had one
- WHEN the operator reconciles
- THEN nothing is created and nothing is reported

- GIVEN a `RedisFailover` that still sets `networkPolicyNsList`
- WHEN the operator reconciles it
- THEN the failover is managed as though the field were absent, and the operator
  says once, per failover, that the field decides nothing and will be removed

- GIVEN the same failover reconciled every thirty seconds thereafter
- WHEN nothing about it changes
- THEN it is not said again until the operator restarts

- GIVEN a `RedisFailover` written against an earlier release
- WHEN it is applied
- THEN the API still accepts it, so upgrading the operator SHOULD NOT require
  editing any resource

- GIVEN Sentinel addressing instances by name
- WHEN it resolves one
- THEN it uses the cluster's DNS as before, and the operator permits nothing,
  because it no longer writes the rule that denied it

## Constraints

- **A policy left in place keeps enforcing what it last said.** Ceasing to write
  one does not undo it, so the resource has to be deleted. The same omission in
  an earlier release is what
  [#49](https://github.com/powerhome/redis-operator/issues/48) was raised for.
- **The field cannot simply go.** Removing it from the CRD makes the API server
  reject a `RedisFailover` that sets it, which would turn an operator upgrade
  into an edit of every resource that ever used it.
- **Nothing the operator writes reflects the field any more.** Once it decides
  nothing, a reader of the resource has no way to discover that except being
  told.

## Decisions

**Both policies are deleted on every pass, unconditionally.** The alternative is
to look only where the operator believes it once wrote one, which means keeping a
record of that, and the record would be wrong for any failover adopted from an
earlier release or edited by hand. A lookup that finds nothing is the cost, and
it is the same lookup the Redis policy has been paying since it was dropped.

**The warning is said once per failover, per operator process.** Saying it on
every reconcile is a few thousand lines a day for each failover that still sets
the field, which buries the warnings that describe something happening now. Saying
it once and never again would hide it from anyone reading later, so a restart says
it again, which is when someone is usually reading.

A status condition was the other candidate and is worse here. The `STATUS`
printer column shows `.status.conditions[-1:].message`, so a condition about a
deprecated field would replace the health message at a glance, and `AddCondition`
appends whenever the newest condition differs in type, so it would alternate with
`Ready` and write to the API on every pass.

**The field is documented as accepted and ignored where a user meets it.** Its
Go doc comment reaches the CRD schema, so `kubectl explain
redisfailover.spec.networkPolicyNsList` says it is ignored and will be removed.
That covers the reader writing a new resource, who never sees the log line
because no policy of theirs was ever deleted.

**The two destroy paths share one body.** Removing a policy the operator does not
manage is the same act for both, and the Redis one already existed. Two copies of
a get-then-delete that tolerates absence is the kind of duplication that drifts.

## Date

2026-09-29
