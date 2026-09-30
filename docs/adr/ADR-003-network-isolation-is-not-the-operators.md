# ADR-003: Network isolation belongs to the cluster, not to the operator

## Status

Accepted

## Context

The operator wrote a `NetworkPolicy` confining each failover's Sentinels: ingress
only from namespaces the `RedisFailover` listed, egress only to that failover's
own pods and to a resolver.

It was there to stop the Sentinels of one failover joining the Sentinels of
another. Every failover monitors a master named `mymaster`, so two sets that can
see each other's instances gossip about the same name and merge, and a Sentinel
from a stranger can then vote in a failover it has no business in.

Three measurements decide how much that policy was buying.

**Two independent failovers do not merge without it.** Two failovers, three
Sentinels each, no policy anywhere and full reachability between them: each set
knew only its own two peers and its own master, and neither mentioned the other
in `sentinel masters`, `sentinel sentinels` or `sentinel replicas`. A Sentinel
publishes its hello messages only onto instances it monitors and ignores hellos
for a master name it does not hold, so two failovers that share no instance never
meet. The network was never what kept them apart.

**Sharing an instance contaminates immediately, and the operator repairs it.**
Pointing one failover's Sentinel at the other's master, which is what
`bootstrapNode.host` with `allowSentinels` does when the source is another
`RedisFailover`, had all three Sentinels of the other failover adopt it as a peer
within seven seconds. The operator undid it thirty-three seconds later:
`CheckSentinelMonitor` pulled the stray back to its own master, and
`CheckSentinelNumberInMemory` counted a Sentinel too many and reset. The exposure
is real and is bounded by one reconcile.

**Almost nobody had the policy.** It was written only for a failover that set
`networkPolicyNsList`, a field that appeared in no example and nowhere in the
documentation. Every failover that did not set it already ran exactly as this
decision leaves all of them.

Two further facts about the policy itself: its egress rule permitted only the
failover's own pods, so it forbade the one configuration that deliberately
creates a shared instance, and it was the sole reason the operator had to open
the DNS port for Sentinel at all.

## Decision

The operator does not write, own or repair `NetworkPolicy` resources. It removes
the ones earlier releases wrote, and confining these pods is left to whoever runs
the cluster.

`spec.networkPolicyNsList` is kept and ignored so that a `RedisFailover` written
against an earlier release is still accepted, and will be removed in a later
release.

## Consequences

**A misconfiguration that points two failovers at one Redis now contaminates for
up to a reconcile.** Previously the policy made it impossible where it was in
use. The operator detects and repairs it, and says so in its logs, but there is a
window in which a foreign Sentinel can vote. Anyone who needs that window closed
writes a policy of their own, which they can express better than a field with two
string keys could.

**The operator no longer needs egress to a resolver, or any egress rule.** The
reason Sentinel had to be granted the DNS port was that the operator's own policy
denied it. Addressing instances by name (see ADR-002) still requires cluster DNS
to work from these pods; it no longer requires the operator to permit it.

**One fewer resource per failover, and one fewer API surface to maintain.** The
policy had been narrowed twice and had already lost its Redis half. What remained
answered a question the monitoring topology answers better.

**Removal has to delete, not merely stop creating.** A policy left behind keeps
enforcing what it last said. Both policies are therefore removed on every pass,
which is what makes an upgrade take effect rather than freezing the previous
rules in place.

## Date

2026-09-29
