// In-browser "server" for the ssl-domain-exporter demo. The app talks to the API
// through axios, so we install a custom axios ADAPTER (see index.ts) that routes
// requests here instead of hitting the network.
/* eslint-disable @typescript-eslint/no-explicit-any */
import { getDB, saveDB } from "./db";

const FEATURES = { http_check: true, cipher_check: true, ocsp_check: true, crl_check: true, caa_check: true, notifications: true, csv_export: true, timeline_view: true, dashboard_tag_filter: true, structured_logs: true };
const db = () => getDB() as any;

function summary() {
  const c: any = { total: 0, ok: 0, warning: 0, critical: 0, error: 0, unknown: 0 };
  for (const d of db().domains) { c.total++; const s = d.last_check?.overall_status || "unknown"; c[s] = (c[s] || 0) + 1; }
  c.error_domains = db().domains.filter((d: any) => d.last_check?.overall_status === "error").map((d: any) => ({ id: d.id, name: d.name, error: d.last_check?.ssl_check_error || d.last_check?.http_error }));
  return c;
}
function timeline() {
  const out: any[] = [];
  for (const d of db().domains) {
    const ck = d.last_check; if (!ck) continue;
    if (ck.ssl_expiry_days != null) out.push({ domain_id: d.id, name: d.name, kind: "ssl", days: ck.ssl_expiry_days, issuer: ck.ssl_issuer });
    if (ck.domain_expiry_days != null) out.push({ domain_id: d.id, name: d.name, kind: "domain", days: ck.domain_expiry_days });
  }
  return out.sort((a, b) => a.days - b.days);
}
function searchDomains(params: any) {
  let items = [...db().domains];
  if (params.search) { const q = String(params.search).toLowerCase(); items = items.filter((d) => d.name.toLowerCase().includes(q)); }
  if (params.status) items = items.filter((d) => d.last_check?.overall_status === params.status);
  if (params.folder_id != null && params.folder_id !== "") items = items.filter((d) => String(d.folder_id) === String(params.folder_id));
  if (params.tag) items = items.filter((d) => (d.tags || []).includes(params.tag));
  const dir = params.sort_dir === "desc" ? -1 : 1;
  const by = params.sort_by || "custom";
  items.sort((a, b) => {
    if (by === "name") return a.name.localeCompare(b.name) * dir;
    if (by === "ssl_expiry") return ((a.last_check?.ssl_expiry_days ?? 9e9) - (b.last_check?.ssl_expiry_days ?? 9e9)) * dir;
    if (by === "status") return String(a.last_check?.overall_status).localeCompare(String(b.last_check?.overall_status)) * dir;
    return (a.sort_order - b.sort_order) * dir;
  });
  const page = Number(params.page) || 1, page_size = Number(params.page_size) || 25;
  const total = items.length;
  return { items: items.slice((page - 1) * page_size, page * page_size), total, page, page_size, total_pages: Math.max(1, Math.ceil(total / page_size)), sort_by: by, sort_dir: params.sort_dir || "asc" };
}

// route on a path with the /api prefix already stripped ("/domains", "/me", "/health"…)
function route(method: string, p: string, params: any, body: any): any {
  const M = method.toUpperCase();
  const idm = (re: RegExp) => p.match(re);
  if (p === "/health" || p === "/ready") return { status: "ok" };
  if (p === "/bootstrap") return { auth: { enabled: false, public_ui: true, anonymous_read_only: false, mode: "basic" }, prometheus: { enabled: true, path: "/metrics", public: true }, features: FEATURES, alerts: { enabled: true, ssl_warning_days: 21, ssl_critical_days: 7, domain_warning_days: 30 }, domains: { default_check_mode: "full" } };
  if (p === "/me") return { authenticated: true, username: "demo", role: "admin", source: "demo", can_view: true, can_edit: true, can_admin: true, public_ui: true };
  if (p === "/config") return { features: FEATURES, version: "1.0.0", build: "demo" };
  if (p === "/session/login") return { authenticated: true, username: "demo", role: "admin", source: "demo", can_view: true, can_edit: true, can_admin: true, public_ui: true };
  if (p === "/session/logout") return {};
  if (p === "/summary") return summary();
  if (p === "/timeline") return timeline();
  if (p === "/tags") return db().tags;
  if (p === "/folders") return db().folders;
  if (p === "/custom-fields") return db().customFields;
  if (p === "/users") return db().users;
  if (p === "/audit-logs") return { items: db().domains.slice(0, 8).map((d: any, i: number) => ({ id: 900 - i, actor: ["demo", "alice"][i % 2], action: ["domain.check", "domain.update", "domain.create", "login"][i % 4], target: d.name, at: new Date(Date.now() - i * 3600_000).toISOString() })), total: 8 };
  if (p === "/notifications/status") return { channels: [{ type: "slack", name: "#ssl-alerts", enabled: true, configured: true }, { type: "email", name: "ops@acme.io", enabled: true, configured: true }, { type: "webhook", name: "PagerDuty", enabled: false, configured: true }] };
  if (p === "/notifications/test") return { sent: true };
  if (p === "/syslog/test") return { ok: true };
  if (p === "/maintenance/backups") return { items: [{ id: "bk-2026-06-10", created_at: new Date().toISOString(), size: 184320 }] };
  if (p === "/maintenance/backup") return { id: "bk-" + Date.now(), created_at: new Date().toISOString(), size: 184320 };
  if (p === "/maintenance/prune") return { pruned: 0 };
  if (p === "/f5/certificates") return { items: [], message: "No F5 BIG-IP source configured (demo)" };
  if (p === "/k8s/certificates") return { items: db().domains.filter((d: any) => d.source_type === "kubernetes_secret").map((d: any) => ({ namespace: d.source_ref.namespace, secret: d.source_ref.secret, domain: d.name })) };
  if (p === "/domains/export.csv") return ["name,status,ssl_issuer,ssl_expiry_days,folder", ...db().domains.map((d: any) => `${d.name},${d.last_check?.overall_status},${d.last_check?.ssl_issuer},${d.last_check?.ssl_expiry_days},${d.folder_id ?? ""}`)].join("\n");
  if (p === "/domains/search") return searchDomains(params);
  if (p === "/domains/reorder") return undefined;
  if (p === "/domains/import") return { mode: "upsert", dry_run: false, summary: { total: 0, created: 0, updated: 0, skipped: 0, failed: 0 }, results: [] };
  if (p === "/domains" && M === "GET") return db().domains;
  if (p === "/domains" && M === "POST") { const id = Math.max(0, ...db().domains.map((d: any) => d.id)) + 1; const d = { id, name: body?.name || body?.domain || "new.example.com", port: body?.port || 443, enabled: body?.enabled ?? true, check_interval: 3600, tags: Array.isArray(body?.tags) ? body.tags : [], metadata: body?.metadata || {}, source_type: "manual", source_ref: {}, folder_id: body?.folder_id ?? null, sort_order: id, custom_ca_pem: "", check_mode: "full", dns_servers: "", created_at: new Date().toISOString(), updated_at: new Date().toISOString(), last_check: db().domains[0]?.last_check }; db().domains.push(d); saveDB(); return d; }
  let m = idm(/^\/domains\/(\d+)\/check$/); if (m) { const id = Number(m[1]); const d = db().domains.find((x: any) => x.id === id); if (d?.last_check) d.last_check.checked_at = new Date().toISOString(); saveDB(); return d?.last_check; }
  m = idm(/^\/domains\/(\d+)\/history$/); if (m) { const id = Number(m[1]); const d = db().domains.find((x: any) => x.id === id); const base = d?.last_check; const items = base ? Array.from({ length: 8 }, (_, i) => ({ ...base, id: base.id - i, checked_at: new Date(Date.now() - i * 6 * 3600_000).toISOString() })) : []; return { items, total: items.length, page: 1, page_size: 25, total_pages: 1 }; }
  m = idm(/^\/domains\/(\d+)\/notify$/); if (m) return { sent: true };
  m = idm(/^\/domains\/(\d+)$/); if (m) { const id = Number(m[1]); const d = db().domains.find((x: any) => x.id === id); if (M === "GET") return d || { __status: 404 }; if (M === "PUT") { Object.assign(d, body, { updated_at: new Date().toISOString() }); saveDB(); return d; } if (M === "DELETE") { db().domains = db().domains.filter((x: any) => x.id !== id); saveDB(); return {}; } }
  m = idm(/^\/folders\/(\d+)$/); if (m) { const id = Number(m[1]); if (M === "DELETE") { db().folders = db().folders.filter((x: any) => x.id !== id); saveDB(); return {}; } const f = db().folders.find((x: any) => x.id === id); if (M === "PUT") { Object.assign(f, body); saveDB(); } return f; }
  m = idm(/^\/custom-fields\/(\d+)$/); if (m) { const id = Number(m[1]); if (M === "DELETE") { db().customFields = db().customFields.filter((x: any) => x.id !== id); saveDB(); return {}; } const cf = db().customFields.find((x: any) => x.id === id); if (M === "PUT") { Object.assign(cf, body); saveDB(); } return cf; }
  m = idm(/^\/users\/(\d+)$/); if (m) { const id = Number(m[1]); if (M === "DELETE") { db().users = db().users.filter((x: any) => x.id !== id); saveDB(); return {}; } return db().users.find((x: any) => x.id === id); }
  if (p === "/folders" && M === "POST") { const id = Math.max(0, ...db().folders.map((f: any) => f.id)) + 1; const f = { id, name: body?.name || "Folder", domain_count: 0, sort_order: id, created_at: new Date().toISOString(), updated_at: new Date().toISOString() }; db().folders.push(f); saveDB(); return f; }
  if (p === "/custom-fields" && M === "POST") { const id = Math.max(0, ...db().customFields.map((f: any) => f.id)) + 1; const f = { ...body, id, options: body?.options || [], created_at: new Date().toISOString(), updated_at: new Date().toISOString() }; db().customFields.push(f); saveDB(); return f; }
  // default
  return M === "GET" ? [] : { ok: true };
}

export function demoAdapter(config: any): Promise<any> {
  const method = (config.method || "get").toUpperCase();
  const base = config.baseURL || "";
  const rawUrl = config.url || "";
  let path = rawUrl.startsWith("http") ? new URL(rawUrl).pathname : (base + rawUrl);
  path = path.replace(/\/{2,}/g, "/");
  if (path.startsWith("/api/")) path = path.slice(4);
  else if (path === "/api") path = "/";
  path = path.replace(/\?.*$/, "");
  let body = config.data;
  if (typeof body === "string") { try { body = JSON.parse(body); } catch { /* leave string */ } }
  return new Promise((resolve) => {
    let data: any;
    try { data = route(method, path, config.params || {}, body); } catch (e) { data = { error: String(e) }; }
    const status = data && data.__status ? data.__status : 200;
    if (data && data.__status) data = { error: "not found" };
    setTimeout(() => resolve({ data, status, statusText: status === 200 ? "OK" : "Error", headers: {}, config, request: {} }), 40 + Math.random() * 70);
  });
}
