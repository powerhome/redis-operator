# Changelog

Notable changes to the `redis-operator` Helm chart.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and the chart follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The chart's `version` moves independently of the operator's, so a chart-only fix
ships without a fake operator release. `appVersion` names the operator a given
chart version installs. Changes to the operator are recorded in the
[repository changelog](https://github.com/powerhome/redis-operator/blob/master/CHANGELOG.md); this file covers the chart alone.

## Unreleased

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
