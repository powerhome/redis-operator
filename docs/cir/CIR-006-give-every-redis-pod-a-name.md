# CIR-006: Give every Redis pod a name

## Intent

Make the service that names the Redis pods exist for every failover, so that
each pod has a name before anything needs one.

The service governing the Redis StatefulSet is what publishes a DNS record per
pod. The operator created it only where a failover enabled the exporter, and
deleted it everywhere else. The exporter is off by default, so most failovers
had no name for any Redis pod.

Carries out part of
[ADR-004](../adr/ADR-004-the-operator-owns-how-instances-are-addressed.md),
which holds that how an instance can be reached is the operator's to settle.

## Behavior

- GIVEN a failover with `redis.exporter.enabled` unset
- WHEN the operator reconciles it
- THEN the service governing the Redis StatefulSet exists, with no port and no
  scrape annotations

- GIVEN a failover with the exporter enabled
- WHEN the operator reconciles it
- THEN that same service carries the exporter port and the scrape annotations

- GIVEN a Redis pod that is not ready, because it reads its dataset from disk
- WHEN something resolves that pod's name
- THEN the name answers with the pod's address

- GIVEN a failover created under an earlier release, with the exporter off
- WHEN this operator reconciles it
- THEN the operator creates the service, which the earlier release deleted,
  and replaces no pod

- GIVEN a failover created under an earlier release, with the exporter on
- WHEN this operator reconciles it
- THEN the operator updates the service in place and replaces no pod

## Constraints

- **A StatefulSet's `serviceName` is immutable.** The service that names the
  pods has to be the one the set already points at. An existing failover cannot
  move to a service created beside it.
- **Sentinel refuses an address it cannot resolve.** `SENTINEL MONITOR` with an
  unresolvable address answers `ERR Invalid IP address or hostname specified`.
  The operator hands Sentinel a pod address today. This constraint binds the
  round that starts handing it a name. A name no record answers for would
  leave a failover with no failover protection, reporting itself healthy. The
  names have to exist first, which is what this change does.
- **A pod has no record until it is ready**, unless the service publishes
  addresses that are not ready. A Redis reading its dataset from disk is not
  ready, and that is when reaching it by name matters most.
- **The name carries three dots and the resolver completes it.** The record is
  `<pod>.<service>.<namespace>.svc`, four labels, which the cluster's search
  list and `ndots:5` complete. `getaddrinfo` resolves it, which is what Redis and
  Sentinel use. busybox `nslookup` sends the name literally and answers
  `NXDOMAIN`, so it reports a fault that is not there.

## Decisions

- **The service exists whether or not the exporter does.** The exporter
  contributes a port and the annotations that get it scraped. The names are
  what the service is for. Tying them to a feature that is off by default left
  the default failover with no names at all. The alternative was to point the
  StatefulSet at the headless service backing HAProxy's SRV discovery. That
  service serves an existing failover not at all, because `serviceName` cannot
  change. It serves a new one only where the spec asks for HAProxy. The
  operator ensures it inside the HAProxy branch, and deletes it with the rest
  of the HAProxy resources.
- **The service publishes addresses for pods that are not ready.** A pod
  loading its dataset is the pod something is most likely to need to reach. A
  name that answers with nothing during that window is a name no caller can
  depend on. Measured on Kubernetes 1.34, this field decides whether the name
  resolves at all. Without it, the name of a pod that is not ready answers
  nothing. With it, the name answers, and the endpoint reports `ready: true`
  and `serving: false`. The field also keeps a terminating pod
  published, as `ready: true` and `terminating: true`, so that pod's name
  answers for as long as its grace period runs. The cost is that the ready
  condition on this service's endpoints says nothing about readiness. A caller
  that needs to know reads `serving`, or asks Redis.
- **`EnsureNotPresentRedisService` goes, rather than sitting unused.** Nothing
  deletes this service now. Leaving the deletion available invites a later
  caller to use it. What it would delete is the service the StatefulSet names.

## Date

2026-11-18
