// Pristine demo dataset for ssl-domain-exporter — a realistic set of monitored
// domains with full TLS/cert/HTTP/registration check results, folders and tags.
/* eslint-disable @typescript-eslint/no-explicit-any */
import type { Domain, Check } from "../../types";

const iso = (msFromNow: number) => new Date(Date.now() + msFromNow).toISOString();
const D = 86_400_000;

let cid = 5000;
function check(id: number, status: Check["overall_status"], sslDays: number, issuer: string, opts: Partial<Check> = {}): Check {
  cid += 1;
  return {
    id: cid, domain_id: id, checked_at: iso(-((id % 30) + 2) * 60_000),
    domain_status: "registered", domain_registrar: "MarkMonitor Inc.", domain_created_at: iso(-1200 * D),
    domain_expires_at: iso((sslDays > 0 ? 320 : 40) * D), domain_expiry_days: sslDays > 0 ? 320 : 40, domain_check_error: "", domain_source: "rdap",
    ssl_issuer: issuer, ssl_subject: "CN=" + (opts.ssl_subject || "*.acme.io"),
    ssl_valid_from: iso(-((90 - sslDays > 0 ? 90 - sslDays : 5)) * D), ssl_valid_until: iso(sslDays * D), ssl_expiry_days: sslDays, ssl_version: "TLS 1.3", ssl_check_error: status === "error" ? "dial tcp: i/o timeout" : "",
    ssl_chain_valid: status !== "error" && status !== "critical", ssl_chain_length: 3, ssl_chain_error: "",
    ssl_chain_details: [
      { subject: "CN=" + (opts.ssl_subject || "*.acme.io"), issuer, valid_from: iso(-85 * D), valid_to: iso(sslDays * D), is_ca: false, is_self_signed: false },
      { subject: issuer, issuer: issuer.includes("Let's") ? "ISRG Root X1" : "DigiCert Global Root G2", valid_from: iso(-1800 * D), valid_to: iso(1500 * D), is_ca: true, is_self_signed: false },
      { subject: issuer.includes("Let's") ? "ISRG Root X1" : "DigiCert Global Root G2", issuer: "self", valid_from: iso(-3650 * D), valid_to: iso(3650 * D), is_ca: true, is_self_signed: true },
    ],
    http_status_code: status === "error" ? 0 : 200, http_redirects_https: true, http_hsts_enabled: status !== "warning", http_hsts_max_age: "63072000", http_response_time_ms: 80 + (id % 7) * 30, http_final_url: "https://" + (opts.http_final_url || "acme.io") + "/", http_error: status === "error" ? "connection refused" : "",
    cipher_weak: status === "warning", cipher_weak_reason: status === "warning" ? "TLS 1.0 still enabled" : "", cipher_grade: status === "ok" ? "A+" : status === "warning" ? "B" : status === "critical" ? "C" : "F", cipher_details: "TLS_AES_256_GCM_SHA384",
    ocsp_status: status === "error" ? "unknown" : "good", ocsp_error: "", crl_status: "good", crl_error: "",
    caa_present: true, caa: '0 issue "letsencrypt.org"', caa_query_domain: opts.http_final_url || "acme.io", caa_error: "",
    registration_check_skipped: false, registration_skip_reason: "", dns_server_used: "1.1.1.1:53",
    primary_reason_code: status === "ok" ? "all_ok" : status === "warning" ? "ssl_expiring_soon" : status === "critical" ? "ssl_expiring_critical" : "check_failed",
    primary_reason_text: status === "ok" ? "All checks passed" : status === "warning" ? `Certificate expires in ${sslDays} days` : status === "critical" ? `Certificate expires in ${sslDays} days` : "TLS handshake failed",
    status_reasons: status === "ok" ? [] : [{ code: "ssl_expiry", severity: status === "critical" ? "critical" : status === "error" ? "error" : "warning", summary: status === "error" ? "Endpoint unreachable" : `SSL expires in ${sslDays} days` }],
    overall_status: status, check_duration_ms: 120 + (id % 9) * 40,
    ...opts,
  } as Check;
}

function domain(id: number, name: string, status: Check["overall_status"], sslDays: number, issuer: string, folder: number | null, tags: string[]): Domain {
  return {
    id, name, port: 443, enabled: true, check_interval: 3600, tags, metadata: { team: tags[0] || "platform", env: folder === 1 ? "prod" : "internal" },
    source_type: id % 9 === 0 ? "kubernetes_secret" : id % 7 === 0 ? "f5_certificate" : "manual",
    source_ref: id % 9 === 0 ? { namespace: "ingress-nginx", secret: name.replace(/\W/g, "-") + "-tls" } : {},
    folder_id: folder, sort_order: id, custom_ca_pem: "", check_mode: "full", dns_servers: "", created_at: iso(-(200 - id) * D), updated_at: iso(-1 * D),
    last_check: check(id, status, sslDays, issuer, { http_final_url: name, ssl_subject: name }),
  };
}

export function buildSeed() {
  const domains: Domain[] = [
    domain(1, "acme.io", "ok", 74, "Let's Encrypt R3", 1, ["prod", "web"]),
    domain(2, "www.acme.io", "ok", 74, "Let's Encrypt R3", 1, ["prod", "web"]),
    domain(3, "api.acme.io", "warning", 11, "Let's Encrypt R3", 1, ["prod", "api"]),
    domain(4, "shop.acme.io", "critical", 3, "DigiCert TLS RSA SHA256 2020 CA1", 1, ["prod", "payments"]),
    domain(5, "checkout.acme.io", "ok", 210, "DigiCert TLS RSA SHA256 2020 CA1", 1, ["prod", "payments"]),
    domain(6, "blog.acme.io", "ok", 58, "Let's Encrypt R3", 3, ["marketing"]),
    domain(7, "status.acme.io", "ok", 88, "Google Trust Services WE1", 3, ["ops"]),
    domain(8, "vpn.acme.io", "error", 0, "Internal CA", 2, ["internal", "infra"]),
    domain(9, "grafana.internal.acme.io", "warning", 19, "Internal CA", 2, ["internal", "monitoring"]),
    domain(10, "vault.internal.acme.io", "ok", 312, "Internal CA", 2, ["internal", "security"]),
    domain(11, "legacy.acme.io", "critical", -4, "DigiCert TLS RSA SHA256 2020 CA1", null, ["legacy"]),
    domain(12, "cdn.acme.io", "ok", 140, "Amazon RSA 2048 M02", 1, ["prod", "cdn"]),
    domain(13, "mail.acme.io", "ok", 47, "Let's Encrypt R3", 3, ["ops", "email"]),
    domain(14, "dev.acme.io", "ok", 81, "Let's Encrypt R3", 2, ["internal", "dev"]),
  ];
  return {
    domains,
    folders: [
      { id: 1, name: "Production", domain_count: domains.filter((d) => d.folder_id === 1).length, sort_order: 0, created_at: iso(-200 * D), updated_at: iso(-1 * D) },
      { id: 2, name: "Internal", domain_count: domains.filter((d) => d.folder_id === 2).length, sort_order: 1, created_at: iso(-200 * D), updated_at: iso(-1 * D) },
      { id: 3, name: "Marketing & Ops", domain_count: domains.filter((d) => d.folder_id === 3).length, sort_order: 2, created_at: iso(-200 * D), updated_at: iso(-1 * D) },
    ],
    tags: ["prod", "web", "api", "payments", "internal", "infra", "monitoring", "security", "marketing", "ops", "email", "dev", "cdn", "legacy"],
    customFields: [
      { id: 1, key: "owner", label: "Owner team", type: "text", required: false, placeholder: "team-name", help_text: "Owning team", sort_order: 0, visible_in_table: true, visible_in_details: true, visible_in_export: true, filterable: true, enabled: true, options: [], created_at: iso(-100 * D), updated_at: iso(-100 * D) },
      { id: 2, key: "tier", label: "Service tier", type: "select", required: false, placeholder: "", help_text: "", sort_order: 1, visible_in_table: true, visible_in_details: true, visible_in_export: true, filterable: true, enabled: true, options: [{ value: "tier-1", label: "Tier 1" }, { value: "tier-2", label: "Tier 2" }], created_at: iso(-100 * D), updated_at: iso(-100 * D) },
    ],
    users: [
      { id: 1, username: "demo", role: "admin", source: "demo", created_at: iso(-300 * D), last_login_at: iso(-1 * 3600_000) },
      { id: 2, username: "alice", role: "editor", source: "oidc", created_at: iso(-120 * D), last_login_at: iso(-2 * 3600_000) },
      { id: 3, username: "viewer-bot", role: "viewer", source: "api_key", created_at: iso(-60 * D), last_login_at: iso(-5 * D) },
    ],
  };
}
