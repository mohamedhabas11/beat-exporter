# beat-exporter for Prometheus

> **Maintained fork** — upstream [`trustpilot/beat-exporter`](https://github.com/trustpilot/beat-exporter) is archived. This fork is maintained at [`mohamedhabas11/beat-exporter`](https://github.com/mohamedhabas11/beat-exporter) with modern Go, security fixes, and active CI.

[![Docker Pulls](https://img.shields.io/docker/pulls/trustpilot/beat-exporter.svg?maxAge=604800)](https://hub.docker.com/r/trustpilot/beat-exporter/)

Exposes (file|metric)beat statistics from the Beat HTTP statistics endpoint to Prometheus format, automatically configuring collectors for the appropriate beat type.

## Current coverage

- `filebeat` — full
- `metricbeat` — full
- `packetbeat` — partial
- `auditbeat` — partial
- `heartbeat` / `journalbeat` / `winlogbeat` — via generic `beat`/`libbeat` collectors (heartbeat-specific events planned)

## Setup

Edit your `*beat` configuration and add:

```yaml
http:
  enabled: true
  host: localhost
  port: 5066
```

This exposes the `(file|metric|* )beat` HTTP API at the given port.

Run beat-exporter:

```sh
./beat-exporter --beat.uris="http://localhost:5066"
# scrape multiple beats:
./beat-exporter --beat.uris="http://localhost:5066,http://localhost:5067,unix:///var/run/filebeat.sock"

# via Docker
docker run -p 9479:9479 ghcr.io/mohamedhabas11/beat-exporter:latest --beat.uris="http://host.docker.internal:5066"
```

Default Prometheus port for beat-exporter is `:9479`. Point Prometheus to `0.0.0.0:9479/metrics` (health at `/health` and `/-/healthy`).

## Configuration reference

```
$ ./beat-exporter -help
Usage of ./beat-exporter:
  -beat.system
        Expose system stats (load, cpu cores) from Beat's /stats
  -beat.timeout duration
        Timeout for trying to get stats from beat. (default 10s)
  -beat.uris string
        Comma-separated list of HTTP API addresses of Beats. Supports http://, https:// and unix:// (e.g. "http://localhost:5066" or "unix:///var/run/filebeat.sock"). (default "http://localhost:5066")
  -tls.certfile string
        TLS cert file if you want to use TLS instead of plain HTTP for the exporter itself
  -tls.keyfile string
        TLS key file if you want to use TLS instead of plain HTTP for the exporter itself
  -version
        Show version and exit
  -web.listen-address string
        Address to listen on for web interface and telemetry. (default ":9479")
  -web.telemetry-path string
        Path under which to expose metrics. (default "/metrics")
```

## Development

```sh
go test ./... -race
go vet ./...
./build.sh        # builds .build/<os>-<arch>/beat-exporter
docker build -t beat-exporter:dev .
```

## Contribution

Please use pull requests and issues. See `.spec/revival.md` for the modernization roadmap and `ward brief` for the current task pool.

