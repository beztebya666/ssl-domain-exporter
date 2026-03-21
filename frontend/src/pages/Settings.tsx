import { useState, useEffect } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import type { LucideIcon } from 'lucide-react'
import { Save, Activity, Bell, Clock, Shield, Lock, SlidersHorizontal, Globe } from 'lucide-react'
import { fetchConfig, updateConfig } from '../api/client'
import type { AppConfig } from '../types'
import { UI_BUILD_ID, UI_VERSION } from '../version'

function Section({ title, icon: Icon, children }: {
  title: string; icon: LucideIcon; children: React.ReactNode
}) {
  return (
    <div className="card space-y-4">
      <h3 className="font-semibold text-white flex items-center gap-2 border-b border-gray-800 pb-3">
        <Icon size={16} className="text-blue-400" /> {title}
      </h3>
      {children}
    </div>
  )
}

function Field({ label, hint, restart, children }: { label: string; hint?: string; restart?: boolean; children: React.ReactNode }) {
  return (
    <div>
      <label className="label">
        {label}
        {restart && <span className="ml-1.5 text-[10px] font-medium text-amber-500 border border-amber-500/30 rounded px-1 py-0.5 align-middle">restart</span>}
      </label>
      {children}
      {hint && <p className="text-xs text-gray-600 mt-1">{hint}</p>}
    </div>
  )
}

function BoolSelect({ value, onChange }: { value: boolean; onChange: (v: boolean) => void }) {
  return (
    <select className="input" value={value ? 'true' : 'false'} onChange={e => onChange(e.target.value === 'true')}>
      <option value="false">Disabled</option>
      <option value="true">Enabled</option>
    </select>
  )
}

export default function SettingsPage() {
  const { data, isLoading } = useQuery({ queryKey: ['config'], queryFn: fetchConfig })
  const [form, setForm] = useState<AppConfig | null>(null)
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    if (data) {
      setForm(data)
    }
  }, [data])

  const mutation = useMutation({
    mutationFn: (payload: Partial<AppConfig>) => updateConfig(payload),
    onSuccess: (next) => {
      setForm(next)
      setSaved(true)
      setTimeout(() => setSaved(false), 2500)
    },
  })

  if (isLoading || !form) {
    return <div className="p-6 text-gray-500">Loading settings...</div>
  }

  const save = () => mutation.mutate(form)

  return (
    <div className="p-6 space-y-5 max-w-5xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-white">Settings</h1>
          <p className="text-sm text-gray-400 mt-0.5">Changes persist to config.yaml immediately. Fields marked <span className="text-amber-500 text-xs font-medium">restart</span> require a service restart to take effect.</p>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-slate-300">UI {UI_VERSION}</span>
            <span className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-slate-400">Build {UI_BUILD_ID}</span>
          </div>
        </div>
        <button className="btn-primary" onClick={save} disabled={mutation.isPending}>
          <Save size={14} />
          {saved ? 'Saved!' : mutation.isPending ? 'Saving...' : 'Save Settings'}
        </button>
      </div>

      <Section title="Checker" icon={Clock}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Database path" hint="SQLite file path" restart>
            <input className="input" value={form.database.path}
              onChange={e => setForm(f => f ? ({ ...f, database: { ...f.database, path: e.target.value } }) : f)} />
          </Field>
          <Field label="Check interval" hint="e.g. 1h, 6h, 24h">
            <input className="input" value={form.checker.interval}
              onChange={e => setForm(f => f ? ({ ...f, checker: { ...f.checker, interval: e.target.value } }) : f)} />
          </Field>
          <Field label="Timeout" hint="e.g. 30s, 1m">
            <input className="input" value={form.checker.timeout}
              onChange={e => setForm(f => f ? ({ ...f, checker: { ...f.checker, timeout: e.target.value } }) : f)} />
          </Field>
          <Field label="Concurrent checks" restart>
            <input className="input" type="number" min={1} max={50} value={form.checker.concurrent_checks}
              onChange={e => setForm(f => f ? ({ ...f, checker: { ...f.checker, concurrent_checks: Number(e.target.value) } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Feature Flags" icon={SlidersHorizontal}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Field label="HTTP/HTTPS checks"><BoolSelect value={form.features.http_check}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, http_check: v } }) : f)} /></Field>
          <Field label="Cipher suite checks"><BoolSelect value={form.features.cipher_check}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, cipher_check: v } }) : f)} /></Field>
          <Field label="OCSP checks"><BoolSelect value={form.features.ocsp_check}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, ocsp_check: v } }) : f)} /></Field>
          <Field label="CRL checks"><BoolSelect value={form.features.crl_check}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, crl_check: v } }) : f)} /></Field>
          <Field label="CAA DNS checks"><BoolSelect value={form.features.caa_check}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, caa_check: v } }) : f)} /></Field>
          <Field label="Notifications"><BoolSelect value={form.features.notifications}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, notifications: v } }) : f)} /></Field>
          <Field label="CSV export"><BoolSelect value={form.features.csv_export}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, csv_export: v } }) : f)} /></Field>
          <Field label="Timeline page"><BoolSelect value={form.features.timeline_view}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, timeline_view: v } }) : f)} /></Field>
          <Field label="Dashboard tag filter"><BoolSelect value={form.features.dashboard_tag_filter}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, dashboard_tag_filter: v } }) : f)} /></Field>
          <Field label="Structured JSON logs" restart><BoolSelect value={form.features.structured_logs}
            onChange={v => setForm(f => f ? ({ ...f, features: { ...f.features, structured_logs: v } }) : f)} /></Field>
        </div>
      </Section>

      <Section title="Alert Thresholds" icon={Shield}>
        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <Field label="SSL warning (days)">
            <input className="input" type="number" value={form.alerts.ssl_expiry_warning_days}
              onChange={e => setForm(f => f ? ({ ...f, alerts: { ...f.alerts, ssl_expiry_warning_days: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="SSL critical (days)">
            <input className="input" type="number" value={form.alerts.ssl_expiry_critical_days}
              onChange={e => setForm(f => f ? ({ ...f, alerts: { ...f.alerts, ssl_expiry_critical_days: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Domain warning (days)">
            <input className="input" type="number" value={form.alerts.domain_expiry_warning_days}
              onChange={e => setForm(f => f ? ({ ...f, alerts: { ...f.alerts, domain_expiry_warning_days: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Domain critical (days)">
            <input className="input" type="number" value={form.alerts.domain_expiry_critical_days}
              onChange={e => setForm(f => f ? ({ ...f, alerts: { ...f.alerts, domain_expiry_critical_days: Number(e.target.value) } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Notifications" icon={Bell}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Field label="Webhook enabled"><BoolSelect value={form.notifications.webhook.enabled}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, webhook: { ...f.notifications.webhook, enabled: v } } }) : f)} /></Field>
          <Field label="Webhook URL">
            <input className="input" value={form.notifications.webhook.url}
              onChange={e => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, webhook: { ...f.notifications.webhook, url: e.target.value } } }) : f)} />
          </Field>
          <Field label="Webhook on critical"><BoolSelect value={form.notifications.webhook.on_critical}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, webhook: { ...f.notifications.webhook, on_critical: v } } }) : f)} /></Field>
          <Field label="Webhook on warning"><BoolSelect value={form.notifications.webhook.on_warning}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, webhook: { ...f.notifications.webhook, on_warning: v } } }) : f)} /></Field>

          <Field label="Telegram enabled"><BoolSelect value={form.notifications.telegram.enabled}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, telegram: { ...f.notifications.telegram, enabled: v } } }) : f)} /></Field>
          <Field label="Telegram bot token">
            <input className="input" value={form.notifications.telegram.bot_token}
              onChange={e => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, telegram: { ...f.notifications.telegram, bot_token: e.target.value } } }) : f)} />
          </Field>
          <Field label="Telegram chat id">
            <input className="input" value={form.notifications.telegram.chat_id}
              onChange={e => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, telegram: { ...f.notifications.telegram, chat_id: e.target.value } } }) : f)} />
          </Field>
          <Field label="Telegram on critical"><BoolSelect value={form.notifications.telegram.on_critical}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, telegram: { ...f.notifications.telegram, on_critical: v } } }) : f)} /></Field>
          <Field label="Telegram on warning"><BoolSelect value={form.notifications.telegram.on_warning}
            onChange={v => setForm(f => f ? ({ ...f, notifications: { ...f.notifications, telegram: { ...f.notifications.telegram, on_warning: v } } }) : f)} /></Field>
        </div>
      </Section>

      <Section title="Auth" icon={Lock}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Auth enabled"><BoolSelect value={form.auth.enabled}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, enabled: v } }) : f)} /></Field>
          <Field label="Mode">
            <select className="input" value={form.auth.mode}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, mode: e.target.value as AppConfig['auth']['mode'] } }) : f)}>
              <option value="basic">basic</option>
              <option value="api_key">api_key</option>
              <option value="both">both</option>
            </select>
          </Field>
          <Field label="Username">
            <input className="input" value={form.auth.username}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, username: e.target.value } }) : f)} />
          </Field>
          <Field label="Password">
            <input className="input" value={form.auth.password}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, password: e.target.value } }) : f)} />
          </Field>
          <Field label="API key">
            <input className="input" value={form.auth.api_key}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, api_key: e.target.value } }) : f)} />
          </Field>
          <Field label="Protect /api"><BoolSelect value={form.auth.protect_api}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_api: v } }) : f)} /></Field>
          <Field label="Protect /metrics"><BoolSelect value={form.auth.protect_metrics}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_metrics: v } }) : f)} /></Field>
          <Field label="Protect UI routes"><BoolSelect value={form.auth.protect_ui}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_ui: v } }) : f)} /></Field>
        </div>
      </Section>

      <Section title="DNS Resolution" icon={Globe}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Global DNS servers" hint="Comma-separated, e.g. 10.0.0.1:53, 10.0.0.2:53">
            <input className="input" value={(form.dns?.servers ?? []).join(', ')}
              onChange={e => setForm(f => f ? ({ ...f, dns: { ...f.dns, servers: e.target.value.split(',').map(s => s.trim()).filter(Boolean) } }) : f)} />
          </Field>
          <Field label="Use system DNS" hint="Fall back to OS-configured resolvers">
            <BoolSelect value={form.dns?.use_system_dns ?? false}
              onChange={v => setForm(f => f ? ({ ...f, dns: { ...f.dns, use_system_dns: v } }) : f)} />
          </Field>
          <Field label="DNS timeout" hint="e.g. 5s, 10s">
            <input className="input" value={form.dns?.timeout ?? '5s'}
              onChange={e => setForm(f => f ? ({ ...f, dns: { ...f.dns, timeout: e.target.value } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Domain Lookup" icon={Globe}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Default check mode" hint="Applied to new domains when no mode is specified">
            <select className="input" value={form.domains.default_check_mode ?? 'full'}
              onChange={e => setForm(f => f ? ({ ...f, domains: { ...f.domains, default_check_mode: e.target.value } }) : f)}>
              <option value="full">Full (SSL + Domain Registration)</option>
              <option value="ssl_only">SSL Only (skip RDAP/WHOIS)</option>
            </select>
          </Field>
          <Field label="Subdomain fallback"><BoolSelect value={form.domains.subdomain_fallback}
            onChange={v => setForm(f => f ? ({ ...f, domains: { ...f.domains, subdomain_fallback: v } }) : f)} /></Field>
          <Field label="Fallback depth" hint="How many parent levels to try">
            <input className="input" type="number" min={1} max={10} value={form.domains.fallback_depth}
              onChange={e => setForm(f => f ? ({ ...f, domains: { ...f.domains, fallback_depth: Number(e.target.value) } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Prometheus & Logging" icon={Activity}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Prometheus enabled" restart><BoolSelect value={form.prometheus.enabled}
            onChange={v => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, enabled: v } }) : f)} /></Field>
          <Field label="Prometheus path" restart>
            <input className="input" value={form.prometheus.path}
              onChange={e => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, path: e.target.value } }) : f)} />
          </Field>
          <Field label="JSON logs" restart><BoolSelect value={form.logging.json}
            onChange={v => setForm(f => f ? ({ ...f, logging: { ...f.logging, json: v } }) : f)} /></Field>
        </div>
      </Section>
    </div>
  )
}
