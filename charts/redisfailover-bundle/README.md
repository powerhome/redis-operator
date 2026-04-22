# `redisfailover-bundle`

Compatibility chart for installing the Redis operator and rendering one or more
`RedisFailover` resources from the values structure currently used by
application deploys such as `example-rails-app`.

This chart assumes the cluster prerequisites already exist:

- the `RedisFailover` CRD is already installed
- the `redis-operator` service account already exists in the target namespace
- the required RBAC for that service account is already installed

## What it installs

- Redis operator deployment, metrics service, and operator `ServiceMonitor`
- `RedisFailover` resources for each `redis.<instance>` entry
- Redis and HAProxy `ServiceMonitor` resources per instance
- A bootstrap `Service` per instance for cross-cluster replication

## Values mapping

The chart preserves the same top-level values contract used by existing app
deployment charts:

- `operators.redis`
  Enables the operator install.
- `applicationName`
  Used in labels and namespaced Redis instance identity.
- `environment`
  Combined with `applicationName` as `<applicationName>-<environment>`.
- `cluster.name`
  Current cluster name, used for replica bootstrap decisions.
- `cluster.replicaCluster`
  Replica cluster name, used to decide whether `bootstrapNode.enabled` is true.
- `serviceMonitor.prometheus`
  Copied onto operator and Redis-related `ServiceMonitor` labels.
- `redis.defaultInstance`
  Preserved for app compatibility; ignored when rendering `RedisFailover`
  instances.
- `redis.<instance>.enabled`
  Enables or disables a specific Redis instance.
- `redis.<instance>.port`
  Mapped to `spec.redis.port`.
- `redis.<instance>.replication`
  When set to `"enabled"`, emits `spec.bootstrapNode`.
- `redis.<instance>.replicaCount.haproxy`
  Mapped to `spec.haproxy.replicas`.
- `redis.<instance>.replicaCount.sentinel`
  Mapped to `spec.sentinel.replicas`.
- `redis.<instance>.replicaCount.redis`
  Mapped to `spec.redis.replicas`.
- `redis.<instance>.resources`
  Merged onto `spec.redis.resources`.
- `redis.<instance>.storage.storageClass`
  Mapped to the Redis PVC `storageClassName`.
- `redis.<instance>.storage.volumeSize`
  Mapped to the Redis PVC requested storage.
- `redis.<instance>.storage.keepAfterDeletion`
  Mapped to `spec.redis.storage.keepAfterDeletion`.

For backwards compatibility with older values, `keepAfterDeletion` is also read
from `redis.<instance>.keepAfterDeletion` when it is not present under
`storage.keepAfterDeletion`.

## Operator-specific values

The operator deployment is configurable via:

- `operator.image.repository`
- `operator.image.tag`
- `operator.image.digest`
- `operator.image.pullPolicy`
- `operator.resources`
- `operator.securityContext`

If `operator.image.tag` is empty, the chart defaults it to `.Chart.AppVersion`.
