# ADR-004: The operator owns how instances are addressed

## Status

Accepted

## Context

A `RedisFailover` accepts `hostNetwork` and `dnsPolicy` on both its Redis and its
Sentinel settings. They arrived in
[e134ab77](https://github.com/powerhome/redis-operator/commit/e134ab77c13fccdb53394a0320fde0a09636b9f0),
described there as "just forwarded unchanged to the corresponding PodSpec". No
use case was recorded with them, no documentation or example in this repository
mentions them, and `hostNetwork` carried no CRD description at all.

Forwarding them unchanged is the problem. Both decide whether a pod can resolve a
cluster DNS name, and `hostNetwork` decides whether a pod has an address of its
own at all. Addressing Redis and Sentinel instances is the operator's central
job: it tells each Sentinel which master to monitor, tells each replica which
master to follow, and reads back what each instance reports. A field that changes
whether an instance can be addressed is an input to that job, not a pod
attribute to pass along.

`dnsPolicy: ClusterFirst` on a `hostNetwork` pod is the sharp edge. Kubernetes
gives such a pod the node's resolver, not cluster DNS, and
`ClusterFirstWithHostNet` is the opt-in that changes it. The operator fills in
`ClusterFirst` when the field is empty, so a failover that sets only
`hostNetwork` gets pods that cannot resolve a cluster name while the operator has
been told they can.

What decided the matter, rather than the sharp edge alone: operators managing
comparable systems do not offer these fields. Percona XtraDB Cluster and
CloudNativePG expose neither field, and Strimzi exposes `dnsPolicy` as a
validated enum and not `hostNetwork`.

The two projects that address instances by name and let a user override it do so
through a field built for that purpose: Strimzi through `advertisedHost` and
`advertisedHostTemplate`, Percona's MongoDB operator through
`clusterServiceDNSMode`. Neither deduces the addressing mode from a pod's DNS
policy.

## Decision

The operator decides how a Redis or Sentinel instance is addressed. The API does
not accept pod-level fields that change whether that addressing can work.

`hostNetwork` and `dnsPolicy` are therefore removed from both the Redis and the
Sentinel settings, over two releases: one that keeps honouring them while naming
them in the operator's logs, and a later one that removes them.

Where addressing needs to vary, it is named directly, as a field whose subject is
the addressing mode. It is not inferred from how a pod is attached to the network.

## Consequences

**A failover cannot put Redis or Sentinel on the node's network.** Anyone relying
on that for reachability from outside the pod network needs a Service, and the
address a Sentinel reports to clients is the operator's to decide, not something
a pod spec can redirect.

**A failover cannot choose its pods' resolver.** A cluster whose nodes carry
internal DNS that Redis needs loses the escape hatch it had, and would need a
field naming that requirement rather than a DNS policy.

**Two releases, not one.** Honouring the fields while announcing them is what
lets an operator upgrade warn a reader before anything stops working. Removing
them in the same release the warning appears would break on upgrade with the
warning arriving too late to act on.

**The rule applies to fields not yet proposed.** A pod attribute that changes
whether an instance can be addressed is refused on this basis, whatever it is
called, which is the point of recording it rather than only deleting two fields.
