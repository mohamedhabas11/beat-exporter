# Helm chart for beat-exporter

## Purpose
Provide a community-grade, installable Helm chart so operators can deploy the
maintained `beat-exporter` fork into Kubernetes with one command, including a
Prometheus Operator `ServiceMonitor` for auto-scraping.

## Signals
- Existing deployment contract: container listens on `:9479`, metrics at
  `/metrics`, health at `/health` and `/-/healthy` (see `main.go`, `readme.md`).
- Published image: `ghcr.io/mohamedhabas11/beat-exporter` (per `readme.md`).
- Configurable flags mirror the binary: `--beat.uris`, `--beat.system`,
  `--beat.timeout`, `--web.listen-address`, `--web.telemetry-path`,
  `--tls.certfile`, `--tls.keyfile` (see `main.go` flag block).

## What's kept
- Chart layout follows the standard Helm v3+ convention (Chart.yaml, values.yaml,
  templates/, _helpers.tpl).
- Non-root distroless image -> pod and container run as non-root by default.
- Probes map to the existing `/health` and `/-/healthy` endpoints.

## What's changed and why
- New `charts/beat-exporter/` tree. Default `image.tag` tracks `appVersion`.
- `serviceMonitor.enabled` (default false) emits a `ServiceMonitor` so the
  Prometheus Operator scrapes `/metrics` without manual config.
- Args are built from values so a single `beat.uris` list drives the deployment.

## Open questions
- Default `beat.uris` points at `http://localhost:5066` (sidecar pattern); should
  the chart also ship a DaemonSet option for host-scoped beats? Defer to v2.
- Repo name on Artifact Hub: publish as `beat-exporter` under the fork org.
