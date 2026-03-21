# SSL Domain Exporter

![image](/docs/screenshots/main_screen.png)

Current version: `v1.2.0`

dockerhub: https://hub.docker.com/r/beztebya666/ssl-domain-exporter

SSL Domain Exporter is a self-hosted service for monitoring SSL certificates (+chain validation), domain registration expiry dates, and DNS health, with a web UI, REST API, SQLite storage, and Prometheus metrics. Designed for enterprise environments including air-gapped networks and internal domains.

## Features

- **SSL checks:** expiry date, issuer/subject, TLS version, certificate chain validation.
- **Domain checks:** RDAP (primary) + WHOIS (fallback), status and registration expiry.
- **Check modes:** `full` (SSL + domain registration) or `ssl_only` (skip RDAP/WHOIS). Per-domain or global default.
- **Custom DNS resolution:** per-domain and global DNS servers with tiered fallback. No silent fallback to public resolvers.
- **Multi-tag support:** each domain can have multiple tags with dedicated UI editing and API support.
- **Structured metadata:** store owner, zone, requester, change ID, environment, or other inventory attributes as key/value metadata.
- **Enterprise import:** line import and JSON batch import with `create_only`, `upsert`, `dry_run`, and optional immediate checks.
- **Optional checks:** HTTP/HTTPS, cipher grade, OCSP, CRL, CAA.
- **Per-domain settings:** HTTPS port, check interval, tags, metadata, folders, custom root CA (PEM), check mode, DNS servers.
- **UI:** Dashboard, domain list, domain details with history, Settings, Timeline (feature flag).
- **CSV export** (feature flag).
- **Webhook and Telegram notifications** on status change to `warning` or `critical`.
- **Prometheus endpoint** with registration-aware metrics and ready-to-use Grafana/Alertmanager files.
- **Cross-platform:** Linux, macOS, and Windows DNS discovery support.

## Check Modes

Each domain has a `check_mode`:

| Mode | SSL | RDAP/WHOIS | HTTP | Cipher | OCSP/CRL | CAA |
|------|-----|------------|------|--------|----------|-----|
| `full` | Yes | Yes | Yes* | Yes* | Yes* | Yes* |
| `ssl_only` | Yes | **Skipped** | Yes* | Yes* | Yes* | Yes* |

\* Optional, controlled by feature flags.

Use `ssl_only` for internal domains (e.g., `.local`, `.internal`) that lack public WHOIS/RDAP records.

When registration check is skipped:
- `domain_expiry_days` metric is removed (not set to 0 or fake value)
- `domain_check_success{type="domain"}` metric is removed (not falsely set to 1)
- `registration_check_skipped` and `registration_skip_reason` are stored in check history for audit
- Notifications exclude domain registration info

## Custom Root CA

Per-domain custom trust roots are still supported through `custom_ca_pem`.

What it does:

- Stores a PEM certificate on the domain record.
- Extends the system trust store for that domain instead of replacing it.
- Is used by both SSL certificate validation and HTTPS checks.
- Lets you monitor internal services signed by a private CA without disabling chain validation.

Where it is available:

- Add domain modal: upload or paste a `.crt` / `.pem` certificate.
- Domain details edit form: update or remove the PEM later.
- REST API: `custom_ca_pem` field on create/update requests.

Behavior notes:

- Empty `custom_ca_pem` means "use default system trust store only".
- Invalid PEM is rejected during checks with `invalid custom_ca_pem certificate`.
- The PEM is treated as an additional root CA, not as a client certificate/key pair.

## Tags and Metadata

Domains now support two separate inventory layers:

- `tags`: multi-value labels such as `prod`, `api`, `customer-facing`
- `metadata`: structured key/value attributes such as `owner=platform-team`, `zone=corp`, `change_id=CHG-001`

Behavior:

- Tags are edited individually in the UI and stored as a normalized array.
- The API accepts legacy tag strings and modern tag arrays.
- Metadata keys are normalized to lowercase and may use letters, numbers, dot, dash, and underscore.
- Empty tag or metadata values are ignored.

Recommended usage:

- Use tags for low-cardinality grouping and filtering.
- Use metadata for richer inventory context that should not become a Prometheus label automatically.

Examples:

- Tags: `["prod", "api", "internal"]`
- Metadata: `{"owner":"platform-team","zone":"corp","change_id":"CHG-042"}`

## Import and IaC

For large environments, use batch import instead of creating domains one by one.

Supported import features:

- `create_only` mode for strict create semantics
- `upsert` mode for idempotent automation / IaC workflows
- `dry_run` for validation without DB writes
- `trigger_checks` to queue immediate checks after import
- `defaults` block for shared settings across all imported domains
- Unknown top-level fields in import items are automatically stored in `metadata`

The web UI supports:

- line import (`one domain per line`)
- JSON import
- shared defaults for tags, metadata, check mode, DNS, interval, folder, custom CA, and enabled state

## DNS Resolution

DNS resolution follows a strict priority chain with no silent public fallback:

1. **Per-domain** DNS servers (`dns_servers` field on domain)
2. **Global** DNS servers (`dns.servers` in config)
3. **System** DNS (OS resolvers, only if `dns.use_system_dns: true`)

If a higher-priority tier fails, the resolver falls back to the next tier.

For TLS/HTTP/cipher/revocation hostname resolution, Go's built-in OS resolver is the final fallback when `use_system_dns` is enabled.

For CAA, Go's stdlib has no native CAA lookup API, so the checker falls back to discovered system DNS servers and common local DNS stubs instead of silently treating CAA as absent.

Custom DNS affects certificate- and HTTP-related hostname resolution plus CAA lookups. It does not currently change RDAP/WHOIS network resolution: those external registration lookups still use the default outbound network stack. For air-gapped or internal-only environments, use `ssl_only`.

On Windows, system DNS servers are discovered via `netsh interface ip show dns`.
On Linux/macOS, they are read from `/etc/resolv.conf`.

The effective DNS server used is recorded in each check as `dns_server_used` for audit trail.

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
- Node.js `20.19+` and npm
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
docker build -t ssl-domain-exporter .
```

Run:

```bash
docker run --name ssl-domain-exporter \
  -p 8080:8080 \
  -v ./data:/app/data \
  -v ./config.yaml:/app/config.yaml \
  ssl-domain-exporter
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

dns:
  servers: []              # Global DNS servers, e.g. ["10.0.0.1:53", "10.0.0.2:53"]
  use_system_dns: true     # Fall back to OS-configured resolvers
  timeout: "5s"

domains:
  subdomain_fallback: true
  fallback_depth: 3
  default_check_mode: "full"   # "full" or "ssl_only"

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
- `POST /api/domains` - accepts `tags` (string or array), `metadata`, `custom_ca_pem`, `check_mode`, `dns_servers`, `enabled`
- `GET /api/domains/:id`
- `PUT /api/domains/:id` - accepts `name` (optional), `tags` (string or array), `metadata`, `custom_ca_pem`, `check_mode`, `dns_servers`, `enabled`; omitted fields keep current values; triggers re-check on significant changes
- `POST /api/domains/import` - batch import API for line/JSON/IaC workflows
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

Create domain with ssl_only mode and custom DNS:

```bash
curl -X POST -u admin:admin \
  -H "Content-Type: application/json" \
  -d '{"name":"internal.local","check_mode":"ssl_only","dns_servers":"10.0.0.1:53"}' \
  http://localhost:8080/api/domains
```

Create domain with a custom root CA:

```bash
curl -X POST -u admin:admin \
  -H "Content-Type: application/json" \
  -d @- http://localhost:8080/api/domains <<'JSON'
{
  "name": "vcenter.internal",
  "custom_ca_pem": "-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
}
JSON
```

Create domain with multiple tags and metadata:

```bash
curl -X POST -u admin:admin \
  -H "Content-Type: application/json" \
  -d @- http://localhost:8080/api/domains <<'JSON'
{
  "name": "api.internal",
  "tags": ["prod", "api", "internal"],
  "metadata": {
    "owner": "platform-team",
    "zone": "corp"
  },
  "check_mode": "ssl_only"
}
JSON
```

Batch import with upsert and metadata:

```bash
curl -X POST -u admin:admin \
  -H "Content-Type: application/json" \
  -d @- http://localhost:8080/api/domains/import <<'JSON'
{
  "mode": "upsert",
  "dry_run": false,
  "trigger_checks": true,
  "defaults": {
    "tags": ["enterprise"],
    "metadata": {
      "zone": "corp"
    },
    "check_mode": "ssl_only"
  },
  "domains": [
    {
      "domain": "vcenter.local",
      "owner": "virtualization-team",
      "change_id": "CHG-001"
    },
    {
      "domain": "api.example.com",
      "tags": ["prod", "api"],
      "metadata": {
        "owner": "web-team"
      },
      "check_mode": "full"
    }
  ]
}
JSON
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

Metrics are exposed at `prometheus.path` (default: `/metrics`).

### Gauge metrics (per domain)

| Metric | Description |
|--------|-------------|
| `domain_ssl_expiry_days` | Days until SSL certificate expires |
| `domain_expiry_days` | Days until domain registration expires (only for `full` mode) |
| `domain_overall_status` | 0=ok, 1=warning, 2=critical, 3=error |
| `domain_ssl_chain_valid` | 1=valid, 0=invalid |
| `domain_check_success{type="ssl\|domain"}` | 1=success, 0=failure |
| `domain_check_duration_ms` | Check duration in milliseconds |
| `domain_last_check_timestamp` | Unix timestamp of last check |
| `domain_registration_check_enabled` | 1=full mode, 0=ssl_only |
| `domain_http_status_code` | Last HTTP status code |
| `domain_http_response_time_ms` | HTTP response time |
| `domain_http_redirects_https` | 1=yes, 0=no for HTTP to HTTPS redirect behavior |
| `domain_http_hsts_enabled` | 1=yes, 0=no for HSTS header presence |
| `domain_cipher_grade` | A=4, B=3, C=2, F=1 |
| `domain_ocsp_status` | good=1, unknown=0, revoked=-1 |
| `domain_crl_status` | good=1, unknown=0, revoked=-1 |
| `domain_caa_present` | 1=yes, 0=no |
| `domain_tag_info{domain,tag}` | Static tag info metric with value `1` for each domain/tag pair |

### Counter metrics

| Metric | Description |
|--------|-------------|
| `domain_checks_total{domain,status}` | Total checks performed |
| `domain_registration_check_skipped_total{domain}` | Checks where RDAP/WHOIS was skipped |

### Important behaviors

- When a domain switches to `ssl_only`, `domain_expiry_days` and `domain_check_success{type="domain"}` are **deleted** (not zeroed)
- When a domain is deleted, all its metric series are cleaned up
- `domain_monitor_total_domains` tracks the total count
- `domain_tag_info` is the recommended metric for Grafana filtering or Prometheus joins by tag
- `domain_expiry_days` may be negative for already expired registrations, which is expected and indicates how many days ago expiry happened

Example PromQL:

```promql
domain_ssl_expiry_days * on(domain) group_left(tag) domain_tag_info{tag="prod"}
```

Note:

- Only tags are exported as Prometheus labels.
- Structured metadata remains available in the UI, REST API, CSV export, and JSON import/export workflows, but is intentionally not exported as free-form metric labels to avoid high-cardinality issues.

Ready-to-use files:
- `monitoring/grafana-dashboard.json`
- `monitoring/alertmanager-rules.yaml`

## Environment Variables

Overrides are supported for most settings, for example:

- `SERVER_HOST`, `SERVER_PORT`, `DATABASE_PATH`
- `AUTH_*` (`AUTH_ENABLED`, `AUTH_MODE`, `AUTH_USERNAME`, `AUTH_PASSWORD`, `AUTH_API_KEY`, ...)
- `FEATURE_*` (`FEATURE_HTTP_CHECK`, `FEATURE_OCSP_CHECK`, ...)
- `PROMETHEUS_ENABLED`, `PROMETHEUS_PATH`
- `WEBHOOK_*`, `TELEGRAM_*`
- `LOG_JSON`
- `DNS_SERVERS` (comma-separated, e.g. `10.0.0.1:53,10.0.0.2:53`)
- `DNS_USE_SYSTEM_DNS` (`true`/`false`)
- `DNS_TIMEOUT` (e.g. `5s`)
- `DEFAULT_CHECK_MODE` (`full` or `ssl_only`)

Full list: `internal/config/config.go` (`applyEnvOverrides`).

## Database Audit Fields

Each check record (`domain_checks` table) stores:

| Field | Description |
|-------|-------------|
| `registration_check_skipped` | Whether RDAP/WHOIS was skipped for this check |
| `registration_skip_reason` | Why it was skipped (e.g. `check_mode=ssl_only`) |
| `dns_server_used` | Effective DNS resolver used (e.g. `per-domain:10.0.0.1:53`) |

## Project Structure

```text
cmd/server            # application entrypoint
internal/api          # HTTP API and auth middleware
internal/checker      # all check logic, scheduler, DNS resolver
internal/config       # config load/validation/save (thread-safe with RWMutex)
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
- Runtime config access is thread-safe: reads go through `Snapshot()`, runtime updates apply via `ApplyFrom()`, and `Save()` writes a normalized snapshot to disk.
- Restart is required for startup-only settings such as `server.host`, `server.port`, `database.path`, `checker.concurrent_checks`, `prometheus.enabled`, `prometheus.path`, `logging.json`, and `features.structured_logs`.
