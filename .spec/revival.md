# spec — beat-exporter revival (archived upstream takeover)

| | |
|---|---|
| Status | Active |
| Domain | revival |
| Version | 1.0.0 |

## Purpose
Trustpilot/beat-exporter is archived; this fork (mohamedhabas11/beat-exporter) becomes the maintained successor. Beat-exporter exposes Elastic Beats `/stats` over HTTP as Prometheus metrics. It is small, useful, and rotten: Go 1.12, prometheus/client_golang v1.3.0, bare global `http.DefaultServeMux`, zero tests, `io/ioutil` deprecation, a global `libbeatOutputType` race, shared `http.Client.Transport` mutation for unix sockets, ancient Dockerfile/GH Actions, and a README that documents a singular `beat.uri` flag that no longer exists (`beat.uris`). Goal: make it trustworthy to run in 2026+ without forking again — modern toolchain, safe concurrency, tested, observable, and documented.

## Signals
- `ward brief` → store clean (0 accepted/verified), no open runs — green field.
- `go 1.26.3` installed locally vs `go 1.12` in go.mod — 14 minor versions drift; `prometheus/common/version` API moved; `io/ioutil` deprecated since Go 1.16.
- `go vet ./...` passes but `go test ./...` → "no test files" in every package — zero coverage.
- `collector/main.go:63` global `var libbeatOutputType *prometheus.Desc` written in `Describe` — race under concurrent scrape.
- `main.go:105-109` mutates `client.Transport` inside loop over URIs — transport is shared, last URI wins.
- `main.go:77` `http.Handle` on global mux; `startHTTPServer` has no graceful shutdown, no context, bare `log.Fatalf`.
- Dockerfile `FROM quay.io/prometheus/busybox:latest` (EOL, runs as root, copies prebuilt `.build/linux-amd64`).
- GH Actions `push.yml`: `actions/setup-go@v1` + Go 1.13 + `checkout@v1` + `upload-artifact@v1` — all deprecated/insecure.
- Readme coverage lists only filebeat/metricbeat/packetbeat/auditbeat partial; heartbeat/journalbeat/winlogbeat missing; flag table missing `beat.uris` comma-list and TLS notes.
- `build.sh` hardcodes `Branch=master` ldflags, builds darwin/linux/windows×amd64/386 with `-a` (slow), no `GO111MODULE` needed now.

## What's kept
- Exporter semantics: one `mainCollector` per `--beat.uris` entry, auto-detecting `BeatInfo.Beat` and registering `beat+libbeat+auditd` plus `filebeat+registrar` or `metricbeat` conditionally, plus optional `system` when `--beat.system` is set. FQNs remain `{{beat}}_{subsystem}_{name}` — churn would break dashboards.
- Collector layout (`collector/*.go` per subsystem: beat, libbeat, filebeat, metricbeat, system, registrar, auditd) — easy to navigate, no mega-file.
- Minimal dependencies: `prometheus/client_golang` + `logrus` + `prometheus/common` — no heavy frameworks.
- MIT license, `trustpilot/beat-exporter` module path preserved for import compatibility (new tags under fork origin).

## What's changed and why
Incremental, each with live verification (`ward task run <id>` captures store-local artifact).

1. **Docs & flag truth (cheap)** — Fix README typos (`appor…`, `(file|metric)beat`), replace `beat.uri` → `beat.uris` (comma-separated), document `unix://` transport, add `tls.*` and `beat.system`. Why: on-call follows docs under incident; stale flags waste hours.

2. **Go vet / ioutil / fmt hygiene (cheap)** — Replace `io/ioutil.ReadAll` → `io.ReadAll`, `ioutil` import → `io`, vet-clean imports, `gofmt`. Why: eliminated deprecation warnings that mask real vet errors.

3. **Toolchain + deps upgrade (mid)** — `go.mod: go 1.12 → 1.22` (covers 2024 LTS floor, still `go1.26` compatible), bump `prometheus/client_golang` → `v1.22.x`, `prometheus/common` → `v0.62`, `logrus` → `v1.9.x` (or `log/slog` bridge). Update `prometheus/common/version` collector API (`NewCollector` signature changed after v0.8). Why: security fixes (CVE in old client), Go 1.12 no longer builds on modern runners, old version package breaks `go mod tidy`.

4. **Collector race & isolation (mid)** — Make `libbeatOutputType` a field on `libbeatCollector`, not `var`; initialize in constructor, not `Describe`. Ensure `filebeatCollector`/`metricbeatCollector` BuildFQName duplication under same period is addressed (distinct descriptors). Why: `ward-state-machine-writes` / `ward-agent-reliability` — silent data races produce phantom metrics that pass narrow tests but fail under load.

5. **HTTP client transport per-target (mid)** — Clone `http.Client` (or `Transport`) per URI; for `unix://` create dedicated `Transport{DialContext: unix}` + `http.Client{Transport: …}`. Do not mutate shared client. Why: shared mutable transport violates `metrics-attribution` — metric for URI A traced to writer for URI B.

6. **Server mux, graceful shutdown, health (mid)** — Replace global `http.Handle`/`http.ListenAndServe` with `http.NewServeMux` + `http.Server`, wire `context.WithCancel` from `SIGINT/SIGTERM`, `Shutdown(ctx)` with 10s timeout, add `/health` and `/-/healthy`. Return errors, don't `log.Fatalf` inside goroutine. Why: `ward-state-machine-writes` — ignored `ListenAndServe` error hides crash loops; global mux leaks between tests.

7. **Unit tests (strong)** — Add `collector/*_test.go` using `net/http/httptest` to test `fetchStatsEndpoint` JSON hack (`"time":123 → "time":{"ms":123}`), `HackfixRegex`, per-collector `Describe`/`Collect` not panicking, `libbeatOutputType` label, and `main.discoverBeatType` with both `http://` and `unix://`. `go test ./...` must pass. Why: zero tests = no trust boundary; every future change is unguarded.

8. **Dockerfile modern (mid)** — Multi-stage: `golang:1.22-bookworm AS builder` → `gcr.io/distroless/static-debian12:nonroot` or `alpine:3.20`. `CGO_ENABLED=0 GOOS=linux go build -trimpath`. `USER nonroot:nonroot`, `EXPOSE 9479`, `ENTRYPOINT`. Why: `busybox:latest` unpinned, runs as root, requires host `.build` pre-copy.

9. **GitHub Actions modern (mid)** — `actions/checkout@v4`, `setup-go@v5` with `go-version: '1.22'`, `actions/upload-artifact@v4`+`download-artifact@v4`, add `golangci-lint`/`govet`/`go test -race`, Docker build with `Buildx`+QF, caching. Why: v1 actions are Node16-retired and will hard-fail on modern runners.

10. **Build ldflags & version (cheap)** — Update `build.sh` to not hardcode `Branch=master` (use `$GITBRANCH`), drop `GO111MODULE=on` + `-a` + `netgo` legacy tags, add `CGO_ENABLED=0 -trimpath -buildvcs=false` where needed. Sync `create-artifacts.sh` to new `.build` layout. Why: reproducible builds, faster CI.

11. **Optional follow-up (deferred, tagged `future`)** — Heartbeat/winlogbeat collectors, OpenTelemetry exemplar, Helm chart. Not in this increment but tracked as `future` tasks so they don't block revival.

## Open questions
- Stay on `logrus` vs migrate to std `log/slog`? Keep `logrus` for this wave (minimal churn), migrate once `slog` JSON handler parity is validated.
- Preserve module path `github.com/trustpilot/beat-exporter` or retag to `github.com/mohamedhabas11/beat-exporter`? Keep old path for drop-in replacement; add `go.mod` retag in next major if we break API.
- Namespace stability: `BeatInfo.Beat` as dynamic namespace (`filebeat_*`, `metricbeat_*`) is unconventional but dashboard-breaking to change — keep until v2 with explicit `exporter_namespace` flag.
- Unix socket support is implemented but untested in CI — should add `httptest` with unix listener; does packetbeat still need special casing?

