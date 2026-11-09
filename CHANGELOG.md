# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Also check this project's [releases](https://github.com/powerhome/redis-operator/releases) for official release notes.

## Unreleased

### Upgrade note

Upgrading to this release replaces every pod whose template carries a default
image, although no default tag changes. A pod template names an image by
string, and adding a digest rewrites that string. This reaches the Redis and
Sentinel StatefulSets, and the HAProxy Deployment when a failover configures
one. It also reaches an enabled exporter sidecar that names no image of its
own. An exporter default replaces its whole parent pod, even when the main
container names its image explicitly.

Redis pods, the master among them, roll under the gate
[CIR-008](docs/cir/CIR-008-hold-the-master-until-a-failover-could-succeed.md)
describes. Expect a short write interruption while Sentinel promotes a replica.
A failover with `spec.redis.replicas: 1` has no replica to promote, so its only
Redis restarts, and data it holds in an `emptyDir` leaves with the pod.

A cluster that rewrites default image references to a private registry needs
that registry to hold each pinned index or manifest first. It also needs the
child manifests and blobs each one names. A mirror that holds one platform
manifest alone does not answer for the pinned multi-architecture index, and the
pull fails.

A failover that names an image keeps it, so naming every image you run is how
you choose your own restart windows. There are five fields: `spec.redis.image`,
`spec.sentinel.image`, `spec.redis.exporter.image`,
`spec.sentinel.exporter.image` and `spec.haproxy.image`. Redis and Sentinel read
the same default, so a named `spec.redis.image` alone leaves the Sentinels on
the default. An exporter runs as a sidecar in its parent pod, so a named main
image alone does not hold that pod still. A failover that names its own images
also stops receiving a newer Redis from an operator upgrade, with the fixes in
it. That choice carries the upgrade with it.

### Changed

- [Pin each default container image to its digest](https://github.com/powerhome/redis-operator/pull/154). A `RedisFailover` that names no image of its own runs a default, and each default now names a tag and the digest that tag resolves to. A tag is mutable. One operator version could otherwise run different bytes in two failovers created months apart. A registry that repointed a tag would also change what a released operator runs. A failover that names its own image sees no change, and every default keeps the version it already served. The upgrade note above says which pods the new references replace

## [v5.0.0] - 2026-11-05

### Upgrade note

Before you upgrade:

- Apply the custom resource definition from this release. Nothing else applies it, and the paragraph below says what a stale one costs.
- Make room for two more pods for each failover whose `haproxy` block names no count. Each one gains two proxies.
- Start from a healthy failover. No Sentinel can carry out a failover while the Sentinels move, so a master lost in that window stays lost until they return.
- Leave node drains until the upgrade finishes, for the same reason.

The Sentinels move from a Deployment to a StatefulSet, which is what gives each one a name in DNS. The Deployment is removed before the set is created, because both produce pods under the same labels and running them together is six Sentinels agreeing a quorum among themselves. Expect a window with no Sentinel able to carry out a failover: measured between 15 and 46 seconds across two upgrades of a two node failover, bounded above by how long the operator takes to come back and point the new Sentinels at a master.

Clients are not affected by that window. HAProxy picks the master by asking each Redis for `role:master` rather than by asking Sentinel, so it keeps serving throughout. Of 3548 writes through it during a measured upgrade, none were refused while no Sentinel could act. A Sentinel-aware client asking `SENTINEL get-master-addr-by-name` gets nothing while the window lasts.

Apply the custom resource definition from this release yourself. The chart carries it in `crds/`, which Helm installs once and never upgrades, so `helm upgrade` leaves whichever definition installed the chart. A stale definition does not fail. It declares no `status.warnings`, and the API server drops that field and reports nothing. Measured against the `v4.8.0` definition: a failover below a minimum held an empty `status.warnings` for a minute, while the log and the gauge both reported it. The operator also logged the sentence on every reconcile rather than once. It reads back what it wrote to decide whether a warning is new. Note that `kubectl apply -f` on this definition fails with `metadata.annotations: Too long`, so apply it with `kubectl apply --server-side`.

Run one replica of the operator across this upgrade. An operator that runs the Sentinels as a set removes a Deployment it finds, but an older one that finds a set leaves it running, so two of them reconciling the same failover can leave both.

Rolling back to an older operator needs one step by hand. That operator leaves the Sentinel StatefulSet running and creates its own Deployment beside it, so six Sentinels answer under one set of labels. Delete the set with `kubectl delete statefulset rfs-<name>` and the Deployment takes the Sentinels over. Measured by installing `v4.8.0` over this release.

This upgrade does not replace the Redis pods. Their pod template is unchanged, so each keeps its identity and the master keeps its role. Measured across two failovers upgraded from `v4.8.0` on a four node cluster. Every Redis pod kept its UID and its revision, and one Redis reported the master role at every five second poll for eight minutes.

A later change to a `RedisFailover` that does change the Redis pod template rolls the pods under a new rule. Replicas go before the master. Each pod goes only once every Sentinel reports a replica it could promote with that pod gone. A roll can therefore pause with one pod still on the previous template. The operator names the Sentinel it is waiting on. A replica no Sentinel would promote does not hold the master back. The master goes ahead of that pod. The pod follows once a Sentinel holds something to promote in its place, which costs a second election. Expect a short window with no master while that role moves, around twenty seconds on a two node failover.

A failover whose `haproxy` block names no count gains two proxy pods. `haproxy.replicas` had no default, which produced a Deployment of zero replicas: a proxy configured, no pod running, and a service with no endpoints. An `int32` with `omitempty` cannot tell an absent field from a literal `0`. So a failover using `replicas: 0` as an off switch gains two pods as well. A failover with no `haproxy` block still runs no proxy.

Every PodDisruptionBudget now keeps at most all but one of its pods. The Sentinel budget takes its `minAvailable` from the Sentinel quorum rather than from the Redis replica count. At the default of two Redis and three Sentinels, both budgets hold what they held before. A failover of one Redis, one Sentinel or two Sentinels had a budget that permitted no eviction. A drain of a node holding one of those pods waited with no end.

A failover whose replica counts cannot survive the loss of a pod now says so in `status.warnings`. The operator logs each sentence once as a warning. It refuses no count: a failover running on one of them keeps running, and the entry leaves when the count does.

### Added

- [Run the Sentinels as a StatefulSet](https://github.com/powerhome/redis-operator/pull/143). Only a StatefulSet gives a Sentinel pod a name in DNS, which is what the operator needs before it can address an instance by name rather than by whatever address it currently holds. The set replaces the Deployment for every failover, whose Sentinel pods are removed before the set is created. See [CIR-009](docs/cir/CIR-009-run-the-sentinels-as-a-set.md)
- [Replace a Sentinel running an old pod template](https://github.com/powerhome/redis-operator/pull/143). The set is created with `OnDelete`, so the StatefulSet controller will not replace a pod to apply a new template, and a StatefulSet cannot bring up a spare ahead of the one it replaces the way the Deployment it succeeded did. The operator replaces one Sentinel per reconcile, and only once the Sentinels that would remain could still agree a failover, naming the ones it is waiting on. A failover with no more Sentinels than its own quorum is asked only that every Sentinel other than the one being replaced reports the master, because asking for a quorum could never be answered yes. A Sentinel that is not reporting at all is replaced whatever the others say, since it casts no vote, and no Sentinel is replaced while none of them reports. See [CIR-009](docs/cir/CIR-009-run-the-sentinels-as-a-set.md)
- Report a replica count that cannot survive a failure. A failover needs two Redis so that one can be promoted when the master is lost, three Sentinels so the others can elect the one that carries a failover out, which is the minimum Redis documents, and two proxies so losing one leaves the client path served. Nothing said so: the counts were accepted in silence, two rolling update gates carved the small ones out so an update would not wedge, and the Sentinel PodDisruptionBudget assumed they did not exist. The operator now names what it cannot make safe in `status.warnings` and logs the same sentence once, and the entry leaves when the count does. No count is refused, because refusing one would stop reconciling a failover that is running on it today. `kubectl explain redisfailover.spec.redis.replicas` and its Sentinel and HAProxy siblings carry the minimum and the reason. The gauge `redis_operator_controller_replica_count_ok{namespace,name,component}` reads 0 for a component below its minimum and 1 for one that meets it, so a fleet can be swept for them at once. See [CIR-014](docs/cir/CIR-014-support-the-replica-counts-we-say-we-support.md)

### Deprecated

- Publishing the operator image to Docker Hub. `ghcr.io/powerhome/redis-operator` is the registry this project installs from. The Helm chart, the kustomize manifests and the plain deployment examples all name it, and the chart itself is published there. A cluster then needs credentials for one registry rather than two. A later release stops the push to Docker Hub, and `powerhome/redis-operator` gains no tags after it. Both registries carry identical tags, digests and platforms today, so a cluster that pulls from Docker Hub switches by changing the registry host alone. Note that ghcr is not backfilled. `v4.5.0` and every earlier tag exist only on Docker Hub.

### Fixed

- Let a node drain finish on a failover too small to survive it. A disruption budget that keeps a whole set permits no eviction, so a drain of a node holding one of those pods waits with no end rather than failing. That is what one Redis, one Sentinel and two Sentinels reached. Each budget now keeps at most all but one, which changes nothing at two or more Redis or three or more Sentinels. Such a failover cannot survive the loss whatever the budget holds. See [CIR-014](docs/cir/CIR-014-support-the-replica-counts-we-say-we-support.md)
- Run proxies for a failover that asks for them and names no count. `redis.replicas` and `sentinel.replicas` default to three, so `haproxy: {}` reads as "on, with defaults". `haproxy.replicas` had no default, which produced a Deployment of zero replicas: a proxy configured, no pod running, and a service with no endpoints. It defaults to two, which is what every example here configures and what a drain, an eviction or a lost node takes to survive. A failover that names a count of its own keeps it, with one exception. An `int32` with `omitempty` cannot tell an explicit zero from an absent field, so `replicas: 0` reads as unset and becomes two. A failover with no `haproxy` block still runs no proxy. See [CIR-014](docs/cir/CIR-014-support-the-replica-counts-we-say-we-support.md)
- [Replace the master only once the Sentinels could promote a replica](https://github.com/powerhome/redis-operator/pull/144). The rolling update asked each replica whether it had finished syncing, which a replica holding no data answers within milliseconds of being told to replicate. A Sentinel that has just been reset, or has just been given a different master to watch, holds no replica it would promote, so removing the master left the failover with none: Sentinel answered `-failover-abort-no-good-slave`, the surviving replica kept the address of a pod that no longer existed, and the operator seeds a master only when every Redis reports `127.0.0.1`, so neither acted again. No Redis is now replaced until every Sentinel reports a replica it could promote, the master included, and the reconcile that replaces the master ends there rather than going on to reset the Sentinels it just asked. Replacing a replica waits on the same condition, because taking one away removes the candidate the Sentinels would promote. A replica Sentinel reports with `slave-priority 0` does not count, since Sentinel will not promote one. `redis.customConfig` applies to every Redis in a failover, so a `replica-priority 0` there leaves nothing to promote and holds every pod on its current template until the setting changes. See [CIR-008](docs/cir/CIR-008-hold-the-master-until-a-failover-could-succeed.md)
- Keep enough Sentinels through a drain to carry out a failover. The Sentinel PodDisruptionBudget took its `minAvailable` from the Redis replica count, so the default of two Redis and three Sentinels allowed two Sentinels to be evicted at once, leaving one that could neither agree the master was down nor be elected to fail it over. The Sentinel PodDisruptionBudget now keeps a majority of the Sentinels, the same quorum the operator monitors the master with. The quorum a Sentinel starts with also follows the Sentinel count rather than being fixed at 2, which only matched two or three Sentinels.

- [Leave the pod being replaced out of what the Sentinels could promote](https://github.com/powerhome/redis-operator/pull/147). The check that holds a rolling update until a failover could succeed runs before the pod is deleted, so the pod in hand was one of the replicas it counted. Where that pod is the only one a Sentinel would promote, the check passed on its own candidacy and the delete took the candidate with it. That needs three or more Redis, with every replica other than the one being replaced unpromotable: unreachable to the Sentinels, or carrying `replica-priority 0`. The operator now names the pod it is about to delete and that pod does not count, so an update holds until the replacement has synced and the Sentinels have read the master's replica list again. The warning says which Sentinel would be left with nothing to promote once the pod is gone. A failover of one master and one replica is unchanged, and is asked only that the Sentinels hold a promotable replica now, because excluding the one replica would always leave zero and no such failover could ever roll. See [CIR-008](docs/cir/CIR-008-hold-the-master-until-a-failover-could-succeed.md)

- [Reapply the Sentinel settings after a refused monitor](https://github.com/powerhome/redis-operator/pull/147). Pointing a Sentinel at a master runs `SENTINEL REMOVE` first, which drops `down-after-milliseconds`, `failover-timeout` and `parallel-syncs` along with everything else that Sentinel holds for `mymaster`. Those come back only from `spec.sentinel.customConfig`, applied in a pass over every Sentinel, and returning as soon as one Sentinel refused its master skipped that pass for all of them: every Sentinel kept Redis's own `30000` and `180000`, six times longer to notice a failure than the failover asks for, for as long as the refusal lasted. The settings are applied whatever any one Sentinel did with its master, each error names the Sentinel it came from, and a Sentinel that fails `SENTINEL SET` no longer denies the others theirs. A failover with `spec.bootstrapNode` loses the same settings to the same remove, and is corrected the same way.

- [Ask a Sentinel for its master on the Sentinel's own port while bootstrapping](https://github.com/powerhome/redis-operator/pull/147). A failover with `spec.bootstrapNode` checks which master each Sentinel monitors, and the external master's address was passed where the Sentinel's port belongs, so the check dialled every Sentinel at an address it does not listen on. It failed on every pass, and the operator took the repair branch every time: `SENTINEL REMOVE` and `SENTINEL MONITOR` on every Sentinel of every bootstrapped failover, every reconcile, raising `SENTINEL_WRONG_MASTER` alongside. The repair pointed them at the right master, so the fault showed as a failover whose Sentinels were reset perpetually rather than one pointing anywhere wrong. The transposition dates from `70b9697e`, which first allowed Sentinels in bootstrap mode.

### Security

- [Remove trust in the compromised eMudhra certificate authorities from the image](https://github.com/powerhome/redis-operator/pull/149). The private keys of eight eMudhra intermediate certificate authorities have too little randomness. A party that recovers one can issue a certificate for any host. Every published image trusts `emSign ECC Root CA - G3`, which those intermediates chain to. The image installs Alpine `ca-certificates 20261020-r0`, which removes that root. The build compiles the operator with Go 1.26.10, which carries the same fix. Go 1.25 gets no fix. The operator verifies the Kubernetes API with the service account's certificate authority. It uses plain TCP to Redis and Sentinel. So the system trust store guards no connection that the operator makes. See the [eMudhra statement](https://emudhra.com/en/post/certificate-security-incident)

## [v4.8.0] - 2026-10-02

### Deprecated

- [Deprecate `hostNetwork` and `dnsPolicy` on Redis and Sentinel](https://github.com/powerhome/redis-operator/pull/139). All four fields still apply, and the operator now names the ones a `RedisFailover` sets in its logs, once per failover. How a Redis or Sentinel instance can be addressed is the operator's to decide, and these change it: `hostNetwork` leaves a pod without an address of its own, and either field can leave a pod unable to resolve a cluster DNS name. They have no replacement, and a later release removes them. Take them out of your `RedisFailover` now, while they still work. See [ADR-004](docs/adr/ADR-004-the-operator-owns-how-instances-are-addressed.md) and [CIR-010](docs/cir/CIR-010-deprecate-pod-networking-fields.md)

### Fixed

- [Make the Sentinel `parallel-syncs` value the one that runs](https://github.com/powerhome/redis-operator/pull/140). The generated Sentinel configuration declared `parallel-syncs 2`, and every Sentinel ran Redis's own `1`: `SENTINEL REMOVE` drops the per-master settings the first time the operator points a Sentinel at the real master, and `parallel-syncs` was not among the settings reapplied over the wire. Both places now declare `1`, which is the value that has been in force all along, so no failover behaves differently. Raising it is a decision about replica read availability during a failover and is left to a change of its own

## [v4.7.1] - 2026-09-25

### Upgrade note

While bootstrapping, a `RedisFailover` publishes no master. Every one of its Redis pods is a replica of the external node, so the service selecting `redisfailovers-role: master` has no endpoints, where before it resolved to a pod reporting `role:slave`. A client connected there could read but not write; it now gets no endpoint at all. The service selecting `redisfailovers-role: slave` gains the pod it had been omitting, so it lists every replica.

The clients that notice are those connecting to the master service directly. A client reaching Redis through the generated HAProxy is unaffected, because HAProxy resolves the headless `_redis._tcp.redis-<name>` SRV record rather than either role service.

This applies only to a `RedisFailover` that elected its own master before it started bootstrapping. One created with `bootstrapNode` already enabled is unaffected, having never held an election.

### Fixed

- [Label every pod a replica while a failover follows an external master](https://github.com/powerhome/redis-operator/pull/130). While bootstrapping, a `RedisFailover` replicates from a node outside its own stateful set, so none of its pods is its master, but the role labels were left at whatever the last election set. The master service therefore selected a pod reporting `role:slave`, which serves reads and rejects writes, and the replica service omitted that pod and so listed fewer replicas than it had. Only a `RedisFailover` that once elected its own master is affected; one created with `bootstrapNode` already enabled labels every pod a replica correctly.
- [Give the NXDOMAIN resolver hold an explicit unit](https://github.com/powerhome/redis-operator/pull/129). Every hold in the generated `resolvers` block is ten seconds except `hold nx`, written as a bare `10`. HAProxy reads a unitless timer as milliseconds, so NXDOMAIN was granted a ten millisecond grace while every sibling failure was granted ten thousand times more. Once the grace expires HAProxy detaches every server from the SRV record, which empties the backend and severs its connections, so a DNS blip of any length emptied the backend rather than being ridden out. Against an eight second NXDOMAIN blip on HAProxy 3.1.7, the same configuration but for this line purged the backend at five seconds with `hold nx 10` and did not purge at all with `hold nx 10s`.

## [v4.7.0] - 2026-09-15

### Upgrade note

Raising the storage on a `RedisFailover` no longer stops every Redis in it at the same moment. Where the storage driver grows a mounted filesystem, nothing restarts. Where it needs a pod restart before the filesystem follows the volume, the operator restarts them one at a time, replicas before the master.

That restart is a Sentinel failover, and clients reaching Redis through the `rfrm-` master service lose writes while it happens. At the default three replicas we measured about a minute. Clients reaching it through HAProxy lost none. The `rfrm-` service follows a new master only once the operator relabels the pod, which it does on a 30 second loop.

The kustomize manifests and the plain deployment examples install `ghcr.io/powerhome/redis-operator:v4.7.0`. At `v4.6.0` they named the upstream project's image, so anyone who installed from them has been running a different operator.

### Fixed

- [Expand a volume without stopping every Redis at once](https://github.com/powerhome/redis-operator/pull/125), resolving [#81](https://github.com/powerhome/redis-operator/issues/81). Raising `storage.persistentVolumeClaim.spec.resources.requests.storage` replaced the `StatefulSet` in a way that took every Redis pod with it at the same moment. The pods now keep serving while the set is replaced around them. Where the storage driver needs a pod restart before the filesystem follows the volume, the operator restarts them one at a time, replicas before the master.

### Changed

- [Install the operator from `ghcr.io/powerhome/redis-operator`](https://github.com/powerhome/redis-operator/pull/122). The kustomize manifests and the plain deployment examples name ghcr, where the operator image and the Helm chart are both published, so a cluster needs credentials for one registry rather than two. Docker Hub carries the same tags, and remains the only place `v4.5.0` and earlier exist. Both sets of manifests previously installed `quay.io/spotahome/redis-operator`, so an install from either one ran the upstream operator rather than this one.

## [v4.6.0] - 2026-09-04

### Upgrade note

A `RedisFailover` with `auth.secretPath` set restarts once when the operator is upgraded, as its pod template gains a record of which password it was built for. The Redis pods go together rather than one at a time, so expect a minute or two with a few tens of seconds of it without a master. A failover with no password is unaffected. The `latest` tag moves to this release.

### Added

- [Publish a bill of materials and build provenance with the image](https://github.com/powerhome/redis-operator/pull/109)

### Changed

- [Build with Go 1.25 and pin the base images by digest](https://github.com/powerhome/redis-operator/pull/108), resolving [#101](https://github.com/powerhome/redis-operator/issues/101)
- [Apply a password change to a running `RedisFailover` without manual intervention](https://github.com/powerhome/redis-operator/pull/104)
- [Publish the operator image to ghcr.io/powerhome in addition to Docker Hub](https://github.com/powerhome/redis-operator/pull/93)

### Fixed

- [Remove the HAProxy resources when the `haproxy` block is taken out of a `RedisFailover`](https://github.com/powerhome/redis-operator/pull/107)
- [Do not promote a Redis node when the current master could not be inspected](https://github.com/powerhome/redis-operator/pull/106), partially addressing [#100](https://github.com/powerhome/redis-operator/issues/100)
- [HAProxy: authenticate the health check so `auth.secretPath` is usable](https://github.com/powerhome/redis-operator/pull/98)
- [Reject an `auth.secretPath` secret whose `password` field is empty](https://github.com/powerhome/redis-operator/pull/98)
- [Authenticate the connected-slaves health check](https://github.com/powerhome/redis-operator/pull/102), resolving [#99](https://github.com/powerhome/redis-operator/issues/99)

## [v4.5.0] - 2026-08-06

### Changed

- [Make leader-election lease renewal tunable, relax defaults](https://github.com/powerhome/redis-operator/pull/91)
- [Update Kubernetes testing matrix](https://github.com/powerhome/redis-operator/pull/88)

## [v4.4.1] - 2026-03-24

### Fixed
- [Fix unnecessary kube deployment and statefulset generation increment on operator tick](https://github.com/powerhome/redis-operator/pull/86)

## [v4.4.0] - 2025-12-19

### Changed

- [Upgraded the project to Go 1.24, updated the redis-operator client to be compatible with Kubernetes 1.32, and applied minor maintenance fixes including linting and operator manifest adjustments](https://github.com/powerhome/redis-operator/pull/84)

## [v4.3.2] - 2025-12-19

### Changed

- [Haproxy: Fix config deprecation warning to support v3.3+](https://github.com/powerhome/redis-operator/pull/82)
- [Add kube 1.32.2 and 1.33.0 to testing matrix](https://github.com/powerhome/redis-operator/pull/75)
- [Drop support for kube versions 1.22 and older](https://github.com/powerhome/redis-operator/pull/74)

## [v4.3.1] - 2025-07-16

### Fixed

- [Fixed health check logic in bootstrap mode](https://github.com/powerhome/redis-operator/pull/72)

## [v4.3.0] - 2025-06-24

### Changed

- [Haproxy: remove haproxy master resources when bootstrapping](https://github.com/powerhome/redis-operator/pull/70)

## [v4.2.0] - 2025-06-20

### Added

- [Haproxy: add additional service labels](https://github.com/powerhome/redis-operator/pull/66)
- [Haproxy: optionally expose prometheus metrics](https://github.com/powerhome/redis-operator/pull/65)

### Fixed

- [Allow different ports on BootstrapNode and RF](https://github.com/powerhome/redis-operator/pull/64)

## [v4.1.0]  - 2025-01-16

### Changed

- [HAProxy: disconnect clients when node is demoted to replica](https://github.com/powerhome/redis-operator/pull/59)

## [v4.0.0] - 2024-12-05

### Changed

- **BREAKING** [HAProxy: avoid sending traffic to replicas on failover](https://github.com/powerhome/redis-operator/pull/57/). RedisFailovers using `haproxy:` must now use a HAProxy image of v3.1.0 or greater. The operator now uses HAProxy v3.1.0 by default.

## [v3.1.0] - 2024-09-05

- [Automatically recreate StatefulSet after volume expansion](https://github.com/powerhome/redis-operator/pull/55)
- [Upgrade default haproxy image to v2.9.6](https://github.com/powerhome/redis-operator/pull/52)

## [v3.0.0] - 2024-02-26

### Removed

- [Remove HAProxy for redis with role:slave as unnecessary and potentially dangerous](https://github.com/powerhome/redis-operator/pull/50)

Action required:

If your application is using the `rfrs-haproxy-[redisfailvover-name]` service you'll need to use the `rfrs-[redis-failover-name]` service which bypassess HAProxy altogether.

## [v2.1.0] - 2024-02-26

### Fixed

- [In version 2.0.1, the approach to generating network policy by the operator was modified. From v2.0.1 onwards, the operator no longer creates network policy for redis but continues to do it for sentinels. This fix automatically removes any leftover network policy from the namespace, eliminating the need for manual intervention](https://github.com/powerhome/redis-operator/pull/49)

### Changed

- [Add default haproxy image](https://github.com/powerhome/redis-operator/pull/47)
- [Update the default redis version to 7.2.4](https://github.com/powerhome/redis-operator/pull/46)
- [Update metrics exporter images](https://github.com/powerhome/redis-operator/pull/45)

## [v2.0.2] - 2024-02-13

### Fixed

- [Operator detects and attempts to heal excessive replication connections on the master node](https://github.com/powerhome/redis-operator/pull/43). This prevents excessive sentinel resets from the operator when extra-RedisFailvoer replication connnections are present on the "slave" nodes.

## [v2.0.1] - 2024-02-09

### Fixed
- [Sentinels shoud only be allowed to talk to pods belonging to their RedisFailover Custom Resource](https://github.com/powerhome/redis-operator/pull/42).

Update notes:

This update modifies how the operator generates network policies. In version v2.0.0, there were two separate network policies: one for Redis and another for Redis Sentinels. From version v2.0.1 onwards, the operator will only generate a network policy for Sentinels. It is crucial to be aware that following the upgrade to this version, the existing network policy for Redis instances will persist and must be deleted manually.

## [v2.0.0] - 2024-01-18

### Added
- [Set up a new HAProxy service for routing traffic specifically to Redis nodes set as slaves][https://github.com/powerhome/redis-operator/pull/40]

Update notes:

This release will change the labels of the HAProxy deployment resource.
It's important to note that in API version apps/v1, a Deployment's label selector [cannot be changed once it's created](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#label-selector-updates). Therefore, any existing HAProxy deployment placed by an <v2.0.0 version of the redis-operator MUST be deleted so the new deployment with the correct labels and selectors can be recreated by redis-operator v2.0.0+

## [v1.8.0] - 2024-01-16

### Fixed
- [Cleanup Sentinels when bootstrapping and sentinels are not allowed](https://github.com/powerhome/redis-operator/pull/33).
- [Prevent connection throttling for more than 256 concurrent client connections](https://github.com/powerhome/redis-operator/pull/36)

### Added
- [Bootstrap settings - Add an `enabled` parameter to toggle the bootstrap mode on or off for a RedisFailover. (Defaults to `true`)](https://github.com/powerhome/redis-operator/pull/32).
- [Use the new docker bake tooling to build the developer tools image and remove vestigial development targets from the Makefile](https://github.com/powerhome/redis-operator/pull/31).

## [v1.8.0-rc2] - 2023-12-20

### Changed
- [Automate image publishing to the public image repository](https://github.com/powerhome/redis-operator/pull/30)
- [Run CI on the master branch](https://github.com/powerhome/redis-operator/pull/29)

## [v1.8.0-rc1] - 2023-12-15

### Added
- [Add public image repository](https://github.com/powerhome/redis-operator/pull/28)

## [v1.7.1] - 2023-12-14

### Changed
- [Use finer grained NetworkPolicies](https://github.com/powerhome/redis-operator/pull/25)

### Fixed
- [Fix PodDisruptionBudget deprecation warnings on kube 1.21+](https://github.com/powerhome/redis-operator/pull/19)

## [v1.1.0-rc.3] - 2022-01-19
### Changes
- Fixed support for kubernetes <1.21

## [v1.1.0-rc.2] - 2022-01-17
### Changes
- Allow configuration of exporter resource
- Fix persistent volume claim metadata management
- Add arm64,arm,amd64 docker images

Update notes:

Ensure you update the CRD definition since CRD is no longer managed by the operator:
```
kubectl create -f https://raw.githubusercontent.com/spotahome/redis-operator/master/example/redisfailover/basic.yaml
```

## [v1.1.0-rc.1] - 2022-01-12

### Major Changes
- Add bootstrap from node
- Custom Resource Definition management is removed from operator logic. It must be added to the API, helm chart manage it now or can be applied with kubectl
- Upgraded libraries to match kubernetes 1.22
- Enable customization for `terminationGracePeriod`
- Fix support for redis 6.2>
- Fix ClusterRole compatible with openshift
- Improve reiliability on liveness probes
- Enable customization of nodeSelector and Tolerations
- Enable customization for command and args in exporter
- Improve auth handling
- Support priorityclassname

Thanks all contributors: @alecjacobs5401, @andriilahuta, @chusAlvarez, @Perfect-Web, Ilya Lesikov, @bit-cloner, Gregory Farnell, @technoplayer, @ThickDrinkLots, @ese, @identw, @LukeCarrier, @k3daevin, @dkulchinsky, @lucming, @cndoit18, @hoffoo, @chlins, @obsessionsys

## [v1.0.0] - 2020-02-24

### Major changes
- Custom Resource Definition moved to `databases.spotahome.com`
- Rolling updates are aware of cluster topology and nodes roles to follow minimum impact strategy
- Better readiness probes for redis nodes.
- More customizable options for kubernetes objects.
- Better bootstrap times.
- Improve security with password protected redis and security pod policies.
- Redis 5 as default version
- Update dependencies

For detailed changelogs see rc relases

## [v1.0.0-rc.5] - 2020-02-07

### Changes
- Custom annotations for services #216 @alecjacobs5401
- Update redis-exporter #222 @VerosK
- Pod security policy to run as non root #228 @logdnalf
- Custom command renames #234 @logdnalf

### Fix
- Add fsGroup to security context #215 @ese
- Pod disruption budget lower than replicas #229 @tkrop
- Add password support for readiness probes #235 @teamon

## [v1.0.0-rc.4] - 2019-12-17

### Changes
- Update kooper to v0.8.0
- Update kubernetes to v1.15.6
- Add support for `hostNetwork` and `dnsPolicy` in Redis and Sentinel pods #212 @paol

## [v1.0.0-rc.3] - 2019-12-10

### Action required

Since update logic has been moved to operator, `PodManagementPolicy` has been set to `Parallel` in redis statefulSet. This improve bootstrap times.
This field is immutable so to upgrade from previous rc releases you need to delete statefulSets manually.
*Note:* you can use `--cascade=false` flag to avoid disruption, pods will be adopted by the new statefulSet created by the operator.
example: `kubectl delete statefulset --cascade=false rfr-redisfailover`

### Changes

- Move rolling update strategy to redis-operator to be cluster-aware #203 @chusAlvarez
- Readiness probe check nodes belong to the cluster and are synced #206 @chusAlvarez
- Support label propagation filter #195 @adamhf
- Support for sentinel prometheus exporter #207 @shonge

### Fix

- Documentation and examples #204 @shonge
- Add RBAC policy to access secrets #208 @hoffoo

## [v1.0.0-rc.2] - 2019-11-15

### Changes

- Add custom annotations for pods in the CRD `podAnnotations` @alecjacobs5401
- Add redis authentication @hoffoo
- Configurable imagePullSecret @romanfurst
- Configurable imagePullPolicy @mcdiae
- Support for node selector `nodeSelector` @sergeunity

### Fix

- Add RBAC policy for the CRD finalizer @mcanevet
- Examples documentation  @SataQiu @marcemq
- Chart service labels @timmyers
- Memory requests and limits for sentinel @marcemq
- Execution permissions in shutdown script @glebpom
- Makefile uid passthrough @adamhf

## [v1.0.0-rc.1] - 2019-05-10

### Changed

- Minimum Kubernetes version needed is 1.9.
- Custom Resource Definition moved to `databases.spotahome.com`.
- API version moved to v1.
- Standardize labels with the Kubernetes recommended ones.
- Update Kubernetes libraries to 1.11.9.
- Update Kooper to v0.5.1.
- Update Golang used to 1.12.
- Use new versioning standard.

### Fixed

- Chart unused values removed.
- Remove double loops for checking Sentinels data in memory.

## [0.5.8] - 2019-03-26

### Fixed

- Now all errors makes a `redisfailover` be marked as failed on metrics, to prevent that some errors were never alerted.

## [0.5.7] - 2019-03-06

### Added

- Command for Redis and Sentinel containers is now configurable.

### Fixed

- Panic if checking the `StartTime` of a pod that was not started yet (nil pointer exception).

## [0.5.6] - 2019-02-27

### Added

- Add tolerations to Redis and Sentinel pods.

### Changed

- Improve management of `customConfig` so they admit any type of configuration.

## [0.5.5] - 2019-02-19

### Added

- Create flag to disable exporter probes.

### Changed

- Increase default memory.
- Improve readability of code.

## [0.5.4] - 2018-10-15

### Changed

- Improve the checker to make it more resilient.
- Reduce startup time.
- When force one master, choose the oldest one.

## [0.5.3] - 2018-09-18

### Added

- Limit length of redis-failovers name to prevent errors when creating the redis statefulsets.
- Add set as failure on metrics when cannot fix the status of redis/sentinel by the operator.
- Remove the redis-failover from metrics if deleted.

## [0.5.2] - 2018-09-04

### Changed

- Higher `InitialDelaySeconds` probes times.

### Fixed

- Default values for spec and validator (lost when release of 1alpha2 api version).

## [0.5.1] - 2018-09-03

### Added

- Persist Redis data on disk.

## [0.5.0] - 2018-08-24

### Added

- Add redis and sentinel custom configuration array.

### Removed

- A `ConfigMap` name for the custom configuration is no longer available.

## [0.4.1] - 2018-08-17

### Added

- Elect a new master when the master pod is terminated.

## [0.4.0] - 2018-07-18

### Added

- Persistence for Redis data in persistent volumes is now available.

## [0.3.0] - 2018-07-03

### Added

- Make name of the Redis Operator container configurable.

### Changed

- Update kooper to v0.3.0, updating the Kubernetes clients to v1.10.5.

## [0.2.5] - 2018-05-25

### Added

- Add the possibility to use a volumen for redis data.

### Changed

- Use the RedisImage to copy the Sentinel configuration in order to use one image less.

## [0.2.4] - 2018-05-24

### Added

- Add the possibility to set the configMap to be used on both Redis and Sentinel.
- Add the possibility to set the redis/sentinel image.
- Add the possibility to set the redis-exporter image and version.

## [0.2.3] - 2018-04-06

### Added

- Add the possibility to use a `NodeAffinity`.

## [0.2.2] - 2018-04-06

### Added

- Add Prometheus Annotations to Redis Exporter.

## [0.2.1] - 2018-03-28

### Fixed

- Create a init-container on sentinel pods so the sentinel.conf is writable.

## [0.2.0] - 2018-02-19

### Added

- Use [Kooper](https://github.com/spotahome/kooper).
- New API version: `storage.spotahome.com/v1alpha2`.

### Changed

- Simplified metrics.
- New client that allows interaction with the redis failovers created.
- New ensurer that checks all pieces are created.
- New checker and healer that puts the nodes into their expected state.

### Removed

- There is no path for upgrade from <0.2.0. You need to create new resources and delete the deprecated CRD resource with `kubectl delete crd redisfailovers.spotahome.com`.

## [0.1.6] - 2018-02-01

### Added

- Add flag to disable `hardaffinity`.
- Wait for CDR before running operator.

## [0.1.5] - 2018-01-03

### Added

- Ensure scheduling on different nodes.
- Export port for gather metrics.
- Add service to chart.

### Changed

- Change waiters so not blocking multiple edits of same resources.

### Fixed

- Only add the redis exporter container if it does not exists.

## [0.1.4] - 2018-01-02

### Added

- Add timeout on waiters.

### Fixed

- Fix WaitForPod unlimited waiting.

## [0.1.3] - 2017-12-29

### Added

- Add/Delete exporter when updating.

### Changed

- Refactor waiters.
- Change concurrency approach. New default limits.

## [0.1.2] - 2017-12-18

### Fixed

- Change kind of response when calling sentinel.

## 0.1.1 - 2017-12-15

### Added

- Initial open-sourced release


[v1.1.0-rc.1]: https://github.com/spotahome/redis-operator/compare/v1.0.0...v1.1.0-rc.1
[v1.0.0]: https://github.com/spotahome/redis-operator/compare/0.5.8...v1.0.0
[v1.0.0-rc.5]: https://github.com/spotahome/redis-operator/compare/v1.0.0-rc.4...v1.0.0-rc.5
[v1.0.0-rc.4]: https://github.com/spotahome/redis-operator/compare/v1.0.0-rc.3...v1.0.0-rc.4
[v1.0.0-rc.3]: https://github.com/spotahome/redis-operator/compare/v1.0.0-rc.2...v1.0.0-rc.3
[v1.0.0-rc.2]: https://github.com/spotahome/redis-operator/compare/v1.0.0-rc.1...v1.0.0-rc.2
[v1.0.0-rc.1]: https://github.com/spotahome/redis-operator/compare/0.5.8...v1.0.0-rc.1
[0.5.8]: https://github.com/spotahome/redis-operator/compare/0.5.7...0.5.8
[0.5.7]: https://github.com/spotahome/redis-operator/compare/0.5.6...0.5.7
[0.5.6]: https://github.com/spotahome/redis-operator/compare/0.5.5...0.5.6
[0.5.5]: https://github.com/spotahome/redis-operator/compare/0.5.4...0.5.5
[0.5.4]: https://github.com/spotahome/redis-operator/compare/0.5.3...0.5.4
[0.5.3]: https://github.com/spotahome/redis-operator/compare/0.5.2...0.5.3
[0.5.2]: https://github.com/spotahome/redis-operator/compare/0.5.1...0.5.2
[0.5.1]: https://github.com/spotahome/redis-operator/compare/0.5.0...0.5.1
[0.5.0]: https://github.com/spotahome/redis-operator/compare/0.4.1...0.5.0
[0.4.1]: https://github.com/spotahome/redis-operator/compare/0.4.0...0.4.1
[0.4.0]: https://github.com/spotahome/redis-operator/compare/0.3.0...0.4.0
[0.3.0]: https://github.com/spotahome/redis-operator/compare/0.2.5...0.3.0
[0.2.5]: https://github.com/spotahome/redis-operator/compare/0.2.4...0.2.5
[0.2.4]: https://github.com/spotahome/redis-operator/compare/0.2.3...0.2.4
[0.2.3]: https://github.com/spotahome/redis-operator/compare/0.2.2...0.2.3
[0.2.2]: https://github.com/spotahome/redis-operator/compare/0.2.1...0.2.2
[0.2.1]: https://github.com/spotahome/redis-operator/compare/0.2.0...0.2.1
[0.2.0]: https://github.com/spotahome/redis-operator/compare/0.1.6...0.2.0
[0.1.6]: https://github.com/spotahome/redis-operator/compare/0.1.5...0.1.6
[0.1.5]: https://github.com/spotahome/redis-operator/compare/0.1.4...0.1.5
[0.1.4]: https://github.com/spotahome/redis-operator/compare/0.1.3...0.1.4
[0.1.3]: https://github.com/spotahome/redis-operator/compare/0.1.2...0.1.3
[0.1.2]: https://github.com/spotahome/redis-operator/compare/0.1.1...0.1.2
