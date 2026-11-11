# Changelog

Notable changes to the `redis-operator` Helm chart.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and the chart follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The chart's `version` moves independently of the operator's, so a chart-only fix
ships without a fake operator release. `appVersion` names the operator a given
chart version installs. Changes to the operator are recorded in the
[repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md); this file covers the chart alone.

## Unreleased

## [4.8.1] - 2026-11-11

Installs operator `v5.1.0`, up from `v5.0.0`. That release pins each default
container image to its digest, which replaces every pod whose template carries
one. Read the upgrade note in the
[repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md)
before you upgrade.

Nothing else in the chart changed.

## [4.8.0] - 2026-11-05

Installs operator `v5.0.0`, up from `v4.8.0`. That release changes behaviour
anyone upgrading has to know about, and the
[repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md)
carries its upgrade note.

The `RedisFailover` custom resource definition the chart installs now records a
minimum and its reason on `redis.replicas`, `sentinel.replicas` and
`haproxy.replicas`, which `kubectl explain` shows, and declares the
`status.warnings` field the operator writes them to. Nothing else in the chart
changed.

Apply that definition yourself. It lives in `crds/`, which Helm installs once
and never upgrades, so `helm upgrade` leaves whichever definition installed the
chart. A stale one declares no `status.warnings`, and the API server drops that
field and reports nothing, so a replica count below its minimum reaches the log
and the gauge and not the resource. `kubectl apply -f` on this definition fails
with `metadata.annotations: Too long`, so apply it with
`kubectl apply --server-side`.

## [4.7.0] - 2026-10-02

Installs operator `v4.8.0`, up from `v4.7.1`. The `RedisFailover` custom resource
definition the chart installs now records that `hostNetwork` and `dnsPolicy` are
deprecated on both the Redis and the Sentinel settings, which `kubectl explain`
shows. Nothing else in the chart changed. See the
[repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md)
for what that operator release contains.

## [4.6.3] - 2026-09-25

Installs operator `v4.7.1`, up from `v4.7.0`. Nothing else in the chart
changed. See the [repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md)
for what that operator release contains.

## [4.6.2] - 2026-09-15

Installs operator `v4.7.0`, up from `v4.6.0`. Nothing else in the chart
changed. See the [repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md)
for what that operator release contains.

## [4.6.1] - 2026-09-14

Installs operator `v4.6.0`.

### Changed

- The operator image is pulled from `ghcr.io/powerhome/redis-operator`, which is
  also where this chart is published, so a cluster needs credentials for one
  registry rather than two. Docker Hub carries the same tags under
  `powerhome/redis-operator`, and remains the only place `v4.5.0` and earlier
  exist.

## [4.6.0] - 2026-09-11

Installs operator `v4.6.0`. First version published to
`oci://ghcr.io/powerhome/charts/redis-operator`. Supersedes `3.3.0`, which
installed `1.3.0`.

### Added

- The bundled custom resource definition is generated from the operator's API, so
  the chart stops shipping a definition older than the operator it installs.
- The ClusterRole grants `redisfailovers/status` and the `networkpolicies` rules
  the operator needs.

### Changed

- The operator image is `powerhome/redis-operator`, at the version this chart
  declares, rather than an upstream image at a version the fork never released.
