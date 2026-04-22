# `redisfailover-bundle`

Compatibility chart for rendering one or more `RedisFailover` resources from the
values structure currently used by
application deploys such as `example-rails-app`.

This chart assumes the cluster prerequisites already exist:

- the Redis operator is already deployed and ready
- the `RedisFailover` CRD is already installed
- the `redis-operator` service account already exists in the target namespace
- the required RBAC for that service account is already installed

## What it installs

- `RedisFailover` resources for each `redis.<instance>` entry
- Redis and HAProxy `ServiceMonitor` resources per instance
- A bootstrap `Service` per instance for cross-cluster replication

## Values mapping

The chart preserves the same top-level values contract used by existing app
deployment charts:

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

## Deployment model

This chart intentionally does not deploy the operator. If strict ordering matters,
the recommended flow is:

1. deploy the Redis operator chart
2. wait for the operator to be ready
3. deploy `redisfailover-bundle`

For a single-command deploy with explicit release ordering, use the repository
root [helmfile.yaml.gotmpl](/Users/artur.zheludkov/power/redis-operator-master/helmfile.yaml.gotmpl:1).
It installs:

1. `redis-operator`
2. `redisfailover-bundle` with `needs: redis-operator`

Example:

```bash
NAMESPACE=redis \
REDIS_OPERATOR_VALUES=./charts/redisoperator/values.yaml \
REDISFAILOVER_BUNDLE_VALUES=./charts/redisfailover-bundle/values.yaml \
helmfile sync
```
