# ADR-002: Redis instances are addressed by DNS name, not by IP

## Status

Accepted

## Context

Three parties exchange addresses for the Redis nodes of a failover, and each
keeps what it is given.

The operator tells Sentinel which node to monitor. A master tells whoever asks
where its replicas are, repeating what each replica announced about itself, and
that is how Sentinel learns the replica set. Sentinel tells replicas where the
master is when it fails one over, and tells clients where to write.

A pod's IP address answers "where is this reachable now" and nothing more. It
outlives the pod, Kubernetes reissues it, and there is no namespace or workload
in it. An address written down by any of the three can therefore come to name a
different pod, possibly in another namespace, serving another failover. Nothing
in the value says so, so nothing notices: a reconfiguration aimed at a
remembered address lands on whatever holds it now.

A pod's name under the service governing its StatefulSet,
`<pod>.<service>.<namespace>.svc`, carries the namespace and the set. It
resolves to the pod it names, or to nothing while there is no such pod. Nothing
is a failure that is visible and safe; the wrong pod is neither.

Redis has supported this since 6.2, which is below the version this project
requires. Sentinel takes `resolve-hostnames` to accept a name and
`announce-hostnames` to report one, and a replica takes `replica-announce-ip` to
say what it should be called.

## Decision

Redis instances are addressed by DNS name wherever an address is written down
somewhere that outlives the moment: what the operator tells Sentinel to monitor,
and what a replica announces to its master.

Addresses remain where the question is "reach this now": the operator opens its
own connections to pod addresses, and it establishes which pod is the master by
asking each one at its address.

Both forms therefore exist, and the rule for which to use is what the value is
for. A value that will be stored and acted on later is a name. A value used to
dial once, now, is an address. Anything that reads a value written by another
party accepts either form, because which party wrote it decides which form it
is.

## Consequences

**Every DNS record the scheme needs has to exist.** The service governing the
Redis StatefulSet is what publishes a record per pod, so it is created whether
or not anything else wants it, and it publishes addresses for pods that are not
ready. Sentinel refuses an address it cannot resolve, so a missing record is not
a degraded failover but one with no failover protection at all.

**Sentinel has to reach a resolver.** A cluster that denies these pods the DNS
port leaves Sentinel unable to accept any address it is given, and it says so
with `ERR Invalid IP address or hostname specified` rather than by degrading
quietly. Whether that port is open is the cluster's to say: see ADR-003.

**Any check that reads a replication target has to handle both forms.** A
replica answers with whatever it was told, so `master_host` is a name where
Sentinel set it and an address where the operator did. A check that understands
only addresses does not fail loudly on a name: it reads the field as absent, and
absent means "this node has no master", so a replica following the wrong master
passes. This is the failure mode to watch for when adding one.

**What clients receive changes.** `SENTINEL get-master-addr-by-name` answers
with a name. Sentinel-aware clients connect to what Sentinel gives them, so this
costs them nothing, but a client that assumes an address, or one running where
cluster DNS does not resolve, cannot reach the master.

**A failover that supplies its own `redis.command` keeps addresses.** The
operator does not append to a command someone else wrote, so such a replica
announces the address it happens to hold and its master lists it that way.

**Bootstrapping keeps addresses.** The master is outside this cluster, and a
name from this cluster's DNS describes nothing that it, or anything reading its
replica list, can reach.

## Date

2026-09-29
