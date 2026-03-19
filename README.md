# Domain SSL Checker

Current version: `v1.1.0`   

dockerhub: https://hub.docker.com/r/beztebya666/ssl-domain-exporter   

Domain SSL Checker is a self-hosted service for monitoring SSL certificates and domain registration expiry dates, with a web UI, REST API, SQLite storage, and Prometheus metrics.

## Features

- SSL checks: expiry date, issuer/subject, TLS version, certificate chain validation.
- Domain checks: RDAP (primary) + WHOIS (fallback), status and registration expiry.
- Optional checks: HTTP/HTTPS, cipher grade, OCSP, CRL, CAA.
- Per-domain settings: HTTPS port, check interval (seconds), tags, folders, custom root CA (PEM).
- UI: Dashboard, domain list, domain details with history, Settings, Timeline (feature flag).
- CSV export (feature flag).
- Webhook and Telegram notifications on status change to `warning` or `critical`.
- Prometheus endpoint and ready-to-use Grafana/Alertmanager files.

## Check Statuses

- `ok`: no errors, thresholds are not exceeded.
- `warning`: upcoming expiry, invalid chain, HTTP 4xx, `cipher grade=C`, OCSP/CRL unknown, etc.
- `critical`: critical expiry, HTTP 5xx, `cipher grade=F`, revoked in OCSP/CRL.
- `error`: both SSL and domain checks failed at the same time.

## Tech Stack

- Backend: Go 1.21, Gin, SQLite (`modernc.org/sqlite`), Prometheus client.
- Frontend: React + Vite + TypeScript + Tailwind.
- DB: local SQLite file (default: `./data/checker.db`).

## Requirements

- Go `1.21+`
- Node.js `20+` and npm
- Network access (RDAP/WHOIS/DNS/OCSP/CRL requests)

## Quick Start (Local)

### 1) Backend

```bash
go mod tidy
go run ./cmd/server
```

By default, the backend starts on `http://localhost:8080`.

### 2) Frontend (dev)

```bash
cd frontend
npm install
npm run dev
```

Frontend dev server: `http://localhost:5174` (proxies `/api` and `/metrics` to backend).

## Run Backend + Embedded UI on One Port

The backend serves `frontend/dist` as static assets. Build the UI first:

```bash
cd frontend
npm install
npm run build
cd ..
go mod tidy
go run ./cmd/server
```

After that, the UI is available at `http://localhost:8080`.

## Docker

Build image:

```bash
docker build -t domain-ssl-checker .
```

Run:

```bash
docker run --name domain-ssl-checker \
  -p 8080:8080 \
  -v ./data:/app/data \
  -v ./config.yaml:/app/config.yaml \
  domain-ssl-checker
```

Health check:

```bash
curl http://localhost:8080/health
```

## Configuration

Main file: `config.yaml`.

Defaults:

- If `config.yaml` is missing, the app creates it with default values.
- Default DB path: `./data/checker.db`.

Ways to set config path:

- Flag `-config <path>`
- ENV `CONFIG_PATH`
- Flag `-config-dir <dir>` or ENV `CONFIG_DIR` (config path becomes `<dir>/config.yaml`)

Safe-start example:

```yaml
server:
  host: "0.0.0.0"
  port: "8080"

database:
  path: "./data/checker.db"

auth:
  enabled: true
  mode: "basic"
  username: "admin"
  password: "change-me"
  protect_api: true
  protect_metrics: true
  protect_ui: false

prometheus:
  enabled: true
  path: "/metrics"
```

## Authentication

Supported modes:

- `basic`
- `api_key`
- `both`

Behavior is controlled by:

- `auth.protect_api`
- `auth.protect_metrics`
- `auth.protect_ui`

Ways to provide API key:

- Header `X-API-Key: <key>`
- Header `Authorization: Bearer <key>`
- Query `?api_key=<key>`

Important: current UI login supports Basic Auth (`username/password`) only. For `api_key` mode, use an API client or enable `both`.

## REST API (Main Endpoints)

### Service

- `GET /health`
- `GET /metrics` (if enabled)

### Config

- `GET /api/config`
- `PUT /api/config`
- `GET /api/settings` (compat)
- `PUT /api/settings` (compat)

### Domains

- `GET /api/domains`
- `POST /api/domains`
- `GET /api/domains/:id`
- `PUT /api/domains/:id`
- `DELETE /api/domains/:id`
- `POST /api/domains/:id/check`
- `GET /api/domains/:id/history?limit=50`
- `POST /api/domains/reorder`
- `GET /api/domains/export.csv` (only when `features.csv_export=true`)

### Folders

- `GET /api/folders`
- `POST /api/folders`
- `PUT /api/folders/:id`
- `DELETE /api/folders/:id`

### Summary

- `GET /api/summary`

## API Request Examples

Basic auth:

```bash
curl -u admin:admin http://localhost:8080/api/domains
```

API key:

```bash
curl -H "X-API-Key: YOUR_KEY" http://localhost:8080/api/summary
```

Run domain check:

```bash
curl -X POST -u admin:admin http://localhost:8080/api/domains/1/check
```

## Feature Flags

Flags in `config.yaml -> features`:

- `http_check`
- `cipher_check`
- `ocsp_check`
- `crl_check`
- `caa_check`
- `notifications`
- `csv_export`
- `timeline_view`
- `dashboard_tag_filter`
- `structured_logs`

## Prometheus and Monitoring

- Metrics are exposed at `prometheus.path` (default: `/metrics`).
- Examples:
`domain_ssl_expiry_days`, `domain_expiry_days`, `domain_overall_status`, `domain_http_status_code`, `domain_checks_total`.
- Ready-to-use files:
`monitoring/grafana-dashboard.json`
`monitoring/alertmanager-rules.yaml`

## Environment Variables

Overrides are supported for most settings, for example:

- `SERVER_HOST`, `SERVER_PORT`, `DATABASE_PATH`
- `AUTH_*` (`AUTH_ENABLED`, `AUTH_MODE`, `AUTH_USERNAME`, `AUTH_PASSWORD`, `AUTH_API_KEY`, ...)
- `FEATURE_*` (`FEATURE_HTTP_CHECK`, `FEATURE_OCSP_CHECK`, ...)
- `PROMETHEUS_ENABLED`, `PROMETHEUS_PATH`
- `WEBHOOK_*`, `TELEGRAM_*`
- `LOG_JSON`

Full list: `internal/config/config.go` (`applyEnvOverrides`).

## Project Structure

```text
cmd/server            # application entrypoint
internal/api          # HTTP API and auth middleware
internal/checker      # all check logic and scheduler
internal/config       # config load/validation/save
internal/db           # SQLite + migrations
internal/metrics      # Prometheus metrics
frontend              # React UI
monitoring            # Grafana dashboard + Alertmanager rules
data                  # SQLite DB (runtime)
```

## Notes

- Scheduler polls every minute and runs checks according to `domain.check_interval`.
- `checker.interval` and `checker.retry_count` exist in config, but current scheduling behavior relies on per-domain `check_interval`.
- With auth enabled, `/api` protection is on by default (`protect_api: true`).
