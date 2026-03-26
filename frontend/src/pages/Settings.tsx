import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { LucideIcon } from 'lucide-react'
import { Save, Activity, Bell, Clock, Shield, Lock, SlidersHorizontal, Globe, Users } from 'lucide-react'
import { createBackup, createUser, deleteUserAccount, fetchAuditLogs, fetchBackupFiles, fetchConfig, fetchHealth, fetchNotificationStatus, fetchReady, fetchUsers, pruneChecks, testNotifications, updateConfig, updateUserAccount } from '../api/client'
import type { AppConfig, AuditLog, AuthMe, BackupFile, NotificationChannel, NotificationTestRequest, NotificationTestResult, UserAccount, UserWritePayload } from '../types'
import CollapsiblePanel from '../components/CollapsiblePanel'
import CustomFieldManager from '../components/CustomFieldManager'
import NotificationSetupWizard from '../components/NotificationSetupWizard'
import SecretInput from '../components/SecretInput'
import { ListCardSkeleton, PageHeadingSkeleton, TableSkeleton } from '../components/Skeleton'
import { useToast } from '../components/ToastProvider'
import { getErrorMessage } from '../lib/utils'
import { UI_BUILD_ID, UI_VERSION } from '../version'

function Section({ title, icon: Icon, children, defaultOpen = false }: {
  title: string; icon: LucideIcon; children: React.ReactNode; defaultOpen?: boolean
}) {
  return (
    <CollapsiblePanel
      title={title}
      icon={Icon}
      defaultOpen={defaultOpen}
      bodyClassName="space-y-4"
    >
      {children}
    </CollapsiblePanel>
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
    <select className="select" value={value ? 'true' : 'false'} onChange={e => onChange(e.target.value === 'true')}>
      <option value="false">Disabled</option>
      <option value="true">Enabled</option>
    </select>
  )
}

type SettingsPageProps = {
  me?: AuthMe
}

type EditableUser = UserAccount & { password?: string }

const STATUS_POLICY_PRESETS: Array<{
  key: 'strict' | 'balanced' | 'expiry'
  label: string
  description: string
  value: AppConfig['status_policy']
}> = [
  {
    key: 'strict',
    label: 'Strict',
    description: 'Most findings raise the main badge, suitable for maximum validation visibility.',
    value: {
      badge_on_invalid_chain: true,
      badge_on_self_signed: true,
      badge_on_http_probe_error: true,
      badge_on_http_client_error: true,
      badge_on_cipher_warning: true,
      badge_on_ocsp_unknown: true,
      badge_on_crl_unknown: true,
      badge_on_caa_missing: true,
      badge_on_domain_lookup_error: true,
    },
  },
  {
    key: 'balanced',
    label: 'Balanced',
    description: 'Expiry and hard failures stay prominent while softer validation issues can remain advisory-only.',
    value: {
      badge_on_invalid_chain: true,
      badge_on_self_signed: false,
      badge_on_http_probe_error: true,
      badge_on_http_client_error: false,
      badge_on_cipher_warning: false,
      badge_on_ocsp_unknown: false,
      badge_on_crl_unknown: false,
      badge_on_caa_missing: false,
      badge_on_domain_lookup_error: false,
    },
  },
  {
    key: 'expiry',
    label: 'Expiry-focused',
    description: 'Operational badge is driven mainly by expiry, revocation, and hard failures. Validation findings stay in notes.',
    value: {
      badge_on_invalid_chain: false,
      badge_on_self_signed: false,
      badge_on_http_probe_error: false,
      badge_on_http_client_error: false,
      badge_on_cipher_warning: false,
      badge_on_ocsp_unknown: false,
      badge_on_crl_unknown: false,
      badge_on_caa_missing: false,
      badge_on_domain_lookup_error: false,
    },
  },
]

function normalizeConfigArrays(config: AppConfig): AppConfig {
  return {
    ...config,
    warnings: config.warnings ?? [],
    server: {
      ...config.server,
      allowed_origins: config.server.allowed_origins ?? [],
    },
    notifications: {
      ...config.notifications,
      email: {
        ...config.notifications.email,
        to: config.notifications.email.to ?? [],
      },
    },
    dns: {
      ...config.dns,
      servers: config.dns.servers ?? [],
    },
    prometheus: {
      ...config.prometheus,
      labels: {
        ...config.prometheus.labels,
        metadata_keys: config.prometheus.labels.metadata_keys ?? [],
      },
    },
  }
}

export default function SettingsPage({ me }: SettingsPageProps) {
  const qc = useQueryClient()
  const { showToast } = useToast()
  const { data, isLoading } = useQuery({ queryKey: ['config'], queryFn: fetchConfig })
  const { data: users = [] } = useQuery({ queryKey: ['users'], queryFn: fetchUsers })
  const { data: notificationStatus = [] } = useQuery({ queryKey: ['notification-status'], queryFn: fetchNotificationStatus })
  const { data: auditLogs = [] } = useQuery({ queryKey: ['audit-logs'], queryFn: fetchAuditLogs })
  const { data: backupFiles = [] } = useQuery({ queryKey: ['backup-files'], queryFn: fetchBackupFiles })
  const { data: health } = useQuery({ queryKey: ['health'], queryFn: fetchHealth, refetchInterval: 30000 })
  const { data: readiness } = useQuery({ queryKey: ['readiness'], queryFn: fetchReady, refetchInterval: 30000 })
  const [form, setForm] = useState<AppConfig | null>(null)
  const [saved, setSaved] = useState(false)
  const [newUser, setNewUser] = useState<UserWritePayload>({ username: '', role: 'viewer', enabled: true, password: '' })
  const [editableUsers, setEditableUsers] = useState<EditableUser[]>([])
  const [notificationTestResults, setNotificationTestResults] = useState<NotificationTestResult[]>([])
  const [pruneDays, setPruneDays] = useState(30)

  useEffect(() => {
    if (data) {
      const normalized = normalizeConfigArrays(data)
      setForm(normalized)
      setPruneDays(normalized.maintenance.check_retention_days > 0 ? normalized.maintenance.check_retention_days : 30)
    }
  }, [data])

  useEffect(() => {
    setEditableUsers(users.map(user => ({ ...user, password: '' })))
  }, [users])

  const mutation = useMutation({
    mutationFn: (payload: Partial<AppConfig>) => updateConfig(payload),
    onSuccess: (next) => {
      setForm(normalizeConfigArrays(next))
      qc.invalidateQueries({ queryKey: ['bootstrap'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      setSaved(true)
      showToast({ tone: 'success', text: 'Settings saved successfully.' })
      setTimeout(() => setSaved(false), 2500)
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to save settings.') }),
  })

  const createUserMutation = useMutation({
    mutationFn: (payload: UserWritePayload) => createUser(payload),
    onSuccess: () => {
      setNewUser({ username: '', role: 'viewer', enabled: true, password: '' })
      qc.invalidateQueries({ queryKey: ['users'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      showToast({ tone: 'success', text: 'User created.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to create user.') }),
  })

  const updateUserMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: Partial<UserWritePayload> }) => updateUserAccount(id, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      showToast({ tone: 'success', text: 'User updated.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to update user.') }),
  })

  const deleteUserMutation = useMutation({
    mutationFn: deleteUserAccount,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      showToast({ tone: 'success', text: 'User deleted.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to delete user.') }),
  })

  const notificationTestMutation = useMutation({
    mutationFn: (payload: NotificationTestRequest) => testNotifications(payload),
    onSuccess: (results) => {
      setNotificationTestResults(results)
      qc.invalidateQueries({ queryKey: ['notification-status'] })
      const failed = results.filter(result => result.enabled && !result.success)
      showToast({
        tone: failed.length > 0 ? 'error' : 'success',
        text: failed.length > 0 ? 'Some test notifications failed. Review channel status below.' : 'Test notifications sent successfully.',
      })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to send test notifications.') }),
  })
  const backupMutation = useMutation({
    mutationFn: createBackup,
    onSuccess: (file) => {
      qc.invalidateQueries({ queryKey: ['backup-files'] })
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      showToast({ tone: 'success', text: `Backup created: ${file.name}` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to create backup.') }),
  })
  const pruneMutation = useMutation({
    mutationFn: (days: number) => pruneChecks(days),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      showToast({ tone: 'success', text: `Removed ${result.removed} historical checks.` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to prune historical checks.') }),
  })
  const enabledAdminCount = useMemo(
    () => editableUsers.filter(user => user.enabled && user.role === 'admin').length,
    [editableUsers],
  )

  if (isLoading || !form) {
    return (
      <div className="space-y-6 p-6">
        <PageHeadingSkeleton />
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <ListCardSkeleton count={2} />
          <ListCardSkeleton count={2} />
        </div>
        <TableSkeleton rows={6} columns={4} />
      </div>
    )
  }

  const save = () => mutation.mutate(form)
  const activePreset = STATUS_POLICY_PRESETS.find(preset => matchesStatusPolicy(preset.value, form.status_policy))?.key
  const applyStatusPreset = (presetKey: (typeof STATUS_POLICY_PRESETS)[number]['key']) => {
    const preset = STATUS_POLICY_PRESETS.find(item => item.key === presetKey)
    if (!preset) return
    setForm(current => current ? ({ ...current, status_policy: { ...preset.value } }) : current)
    showToast({ tone: 'success', text: `Applied ${preset.label} status policy preset. Save settings to persist it.` })
  }

  return (
    <div className="p-6 space-y-5 max-w-6xl">
      <div className="flex items-center justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-xl font-bold text-white">Administration</h1>
          <p className="text-sm text-gray-400 mt-0.5">
            Enterprise configuration, notification channels, public UI behavior, and local user access.
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-slate-300">UI {UI_VERSION}</span>
            <span className="rounded border border-slate-700 bg-slate-800 px-2 py-1 text-slate-400">Build {UI_BUILD_ID}</span>
            {me?.username && <span className="rounded border border-blue-500/20 bg-blue-500/10 px-2 py-1 text-blue-300">Signed in as {me.username}</span>}
          </div>
        </div>
        <button className="btn-primary" onClick={save} disabled={mutation.isPending}>
          <Save size={14} />
          {saved ? 'Saved!' : mutation.isPending ? 'Saving...' : 'Save Settings'}
        </button>
      </div>

      {form.warnings && form.warnings.length > 0 && (
        <div className="rounded-xl border border-amber-500/30 bg-amber-500/15 px-4 py-3 text-sm text-amber-300">
          <div className="font-medium text-amber-400">Deployment warnings</div>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-amber-300">
            {form.warnings.map(warning => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        </div>
      )}

      <Section title="Checker" icon={Clock} defaultOpen>
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
          <Field label="Retry count" hint="Immediate retries for transient check failures within a single run">
            <input className="input" type="number" min={0} max={10} value={form.checker.retry_count}
              onChange={e => setForm(f => f ? ({ ...f, checker: { ...f.checker, retry_count: Number(e.target.value) } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Feature Flags" icon={SlidersHorizontal} defaultOpen>
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

      <Section title="Status Badge Policy" icon={Shield}>
        <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
          <div className="text-sm font-semibold text-white">Quick presets</div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
            {STATUS_POLICY_PRESETS.map(preset => (
              <button
                key={preset.key}
                type="button"
                className={`rounded-xl border p-3 text-left transition-colors ${activePreset === preset.key ? 'border-blue-500/30 bg-blue-500/10' : 'border-slate-800 bg-slate-900/40 hover:border-slate-700'}`}
                onClick={() => applyStatusPreset(preset.key)}
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="text-sm font-medium text-white">{preset.label}</div>
                  {activePreset === preset.key && <span className="text-[11px] text-blue-300">active</span>}
                </div>
                <div className="mt-2 text-xs text-slate-400">{preset.description}</div>
              </button>
            ))}
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Badge on invalid chain" hint="If disabled, invalid-chain findings stay visible in details/history but do not raise the main badge">
            <BoolSelect value={form.status_policy.badge_on_invalid_chain}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_invalid_chain: v } }) : f)} />
          </Field>
          <Field label="Badge on self-signed cert" hint="Useful for internal PKI environments where self-signed or private roots are expected">
            <BoolSelect value={form.status_policy.badge_on_self_signed}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_self_signed: v } }) : f)} />
          </Field>
          <Field label="Badge on HTTP probe error">
            <BoolSelect value={form.status_policy.badge_on_http_probe_error}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_http_probe_error: v } }) : f)} />
          </Field>
          <Field label="Badge on HTTP 4xx">
            <BoolSelect value={form.status_policy.badge_on_http_client_error}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_http_client_error: v } }) : f)} />
          </Field>
          <Field label="Badge on cipher warning">
            <BoolSelect value={form.status_policy.badge_on_cipher_warning}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_cipher_warning: v } }) : f)} />
          </Field>
          <Field label="Badge on OCSP unknown">
            <BoolSelect value={form.status_policy.badge_on_ocsp_unknown}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_ocsp_unknown: v } }) : f)} />
          </Field>
          <Field label="Badge on CRL unknown">
            <BoolSelect value={form.status_policy.badge_on_crl_unknown}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_crl_unknown: v } }) : f)} />
          </Field>
          <Field label="Badge on missing CAA">
            <BoolSelect value={form.status_policy.badge_on_caa_missing}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_caa_missing: v } }) : f)} />
          </Field>
          <Field label="Badge on domain lookup error" hint="Disable if WHOIS/RDAP lookup issues should stay advisory-only while expiry remains the main operational signal">
            <BoolSelect value={form.status_policy.badge_on_domain_lookup_error}
              onChange={v => setForm(f => f ? ({ ...f, status_policy: { ...f.status_policy, badge_on_domain_lookup_error: v } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Notification Setup" icon={Bell} defaultOpen>
        <NotificationSetupWizard
          form={form}
          setForm={setForm}
          notificationStatus={notificationStatus}
          notificationTestResults={notificationTestResults}
          testing={notificationTestMutation.isPending}
          onSendTest={(channel: NotificationChannel) => notificationTestMutation.mutate({
            channel,
            features: { notifications: form.features.notifications },
            notifications: form.notifications,
          })}
        />
      </Section>

      <Section title="Access & Public UI" icon={Lock}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Auth enabled"><BoolSelect value={form.auth.enabled}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, enabled: v } }) : f)} /></Field>
          <Field label="Mode">
            <select className="select" value={form.auth.mode}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, mode: e.target.value as AppConfig['auth']['mode'] } }) : f)}>
              <option value="basic">basic</option>
              <option value="api_key">api_key</option>
              <option value="both">both</option>
            </select>
          </Field>
          <Field label="Public read-only UI" hint="Anonymous viewing becomes available whenever UI routes or read-only API routes are not protected. Keep Protect UI and Protect /api enabled together if you want login-only viewing.">
            <BoolSelect value={!form.auth.protect_ui}
              onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_ui: !v } }) : f)} />
          </Field>
          <Field label="Legacy admin username">
            <input className="input" value={form.auth.username}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, username: e.target.value } }) : f)} />
          </Field>
          <Field label="Legacy admin password">
            <SecretInput
              value={form.auth.password}
              ariaLabel="Legacy admin password"
              onChange={value => setForm(f => f ? ({ ...f, auth: { ...f.auth, password: value } }) : f)}
            />
          </Field>
          <Field label="API key">
            <SecretInput
              value={form.auth.api_key}
              ariaLabel="API key"
              onChange={value => setForm(f => f ? ({ ...f, auth: { ...f.auth, api_key: value } }) : f)}
            />
          </Field>
          <Field label="Session TTL" hint="e.g. 12h, 24h, 168h">
            <input className="input" value={form.auth.session_ttl}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, session_ttl: e.target.value } }) : f)} />
          </Field>
          <Field label="Session cookie name">
            <input className="input" value={form.auth.cookie_name}
              onChange={e => setForm(f => f ? ({ ...f, auth: { ...f.auth, cookie_name: e.target.value } }) : f)} />
          </Field>
          <Field label="Session cookie secure" hint="Enable when the app is served only over HTTPS">
            <BoolSelect value={form.auth.cookie_secure}
              onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, cookie_secure: v } }) : f)} />
          </Field>
          <Field label="Protect /api"><BoolSelect value={form.auth.protect_api}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_api: v } }) : f)} /></Field>
          <Field label="Protect /metrics"><BoolSelect value={form.auth.protect_metrics}
            onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_metrics: v } }) : f)} /></Field>
          <Field label="Protect UI routes" hint="When enabled, anonymous users will see the login gate">
            <BoolSelect value={form.auth.protect_ui}
              onChange={v => setForm(f => f ? ({ ...f, auth: { ...f.auth, protect_ui: v } }) : f)} />
          </Field>
        </div>
      </Section>

      <Section title="Request Safety" icon={Shield}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="CSRF protection" hint="Applies to cookie-based session writes in the browser">
            <BoolSelect value={form.security.csrf_enabled}
              onChange={v => setForm(f => f ? ({ ...f, security: { ...f.security, csrf_enabled: v } }) : f)} />
          </Field>
          <Field label="Soft rate limiting" hint="Targets login and admin write endpoints only">
            <BoolSelect value={form.security.rate_limit_enabled}
              onChange={v => setForm(f => f ? ({ ...f, security: { ...f.security, rate_limit_enabled: v } }) : f)} />
          </Field>
          <Field label="Login requests" hint="Per-IP limit for /session/login within the login window">
            <input className="input" type="number" min={1} value={form.security.login_requests}
              onChange={e => setForm(f => f ? ({ ...f, security: { ...f.security, login_requests: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Login window" hint="e.g. 5m, 15m">
            <input className="input" value={form.security.login_window}
              onChange={e => setForm(f => f ? ({ ...f, security: { ...f.security, login_window: e.target.value } }) : f)} />
          </Field>
          <Field label="Admin write requests" hint="Per-user/IP limit for POST/PUT/DELETE admin actions">
            <input className="input" type="number" min={1} value={form.security.admin_write_requests}
              onChange={e => setForm(f => f ? ({ ...f, security: { ...f.security, admin_write_requests: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Admin window" hint="e.g. 1m, 5m">
            <input className="input" value={form.security.admin_window}
              onChange={e => setForm(f => f ? ({ ...f, security: { ...f.security, admin_window: e.target.value } }) : f)} />
          </Field>
          <Field label="Allowed UI origins" hint="Comma-separated origins for split UI/API deployments. Empty keeps CORS same-origin only.">
            <input className="input" value={(form.server.allowed_origins ?? []).join(', ')}
              onChange={e => setForm(f => f ? ({ ...f, server: { ...f.server, allowed_origins: e.target.value.split(',').map(s => s.trim()).filter(Boolean) } }) : f)} />
          </Field>
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
            <select className="select" value={form.domains.default_check_mode ?? 'full'}
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

      <Section title="Custom Inventory Fields" icon={Globe}>
        <CustomFieldManager />
      </Section>

      <Section title="Prometheus & Logging" icon={Activity}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Prometheus enabled" restart><BoolSelect value={form.prometheus.enabled}
            onChange={v => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, enabled: v } }) : f)} /></Field>
          <Field label="Prometheus path" restart>
            <input className="input" value={form.prometheus.path}
              onChange={e => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, path: e.target.value } }) : f)} />
          </Field>
          <Field label="Export tag labels" hint="Leave enabled to keep domain_tag_info for every domain/tag pair">
            <BoolSelect value={form.prometheus.labels.export_tags}
              onChange={v => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, labels: { ...f.prometheus.labels, export_tags: v } } }) : f)} />
          </Field>
          <Field label="Export metadata labels" hint="Default stays permissive. Disable or whitelist keys only if Prometheus cardinality becomes a problem">
            <BoolSelect value={form.prometheus.labels.export_metadata}
              onChange={v => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, labels: { ...f.prometheus.labels, export_metadata: v } } }) : f)} />
          </Field>
          <Field label="Metadata key allowlist" hint="Comma-separated. Empty means export all metadata keys.">
            <input className="input" value={(form.prometheus.labels.metadata_keys ?? []).join(', ')}
              onChange={e => setForm(f => f ? ({ ...f, prometheus: { ...f.prometheus, labels: { ...f.prometheus.labels, metadata_keys: e.target.value.split(',').map(s => s.trim()).filter(Boolean) } } }) : f)} />
          </Field>
          <Field label="JSON logs" restart><BoolSelect value={form.logging.json}
            onChange={v => setForm(f => f ? ({ ...f, logging: { ...f.logging, json: v } }) : f)} /></Field>
        </div>
      </Section>

      <Section title="Maintenance & Health" icon={Activity}>
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Field label="Backups directory" hint="Used by admin-triggered sqlite backups and CLI backup workflows">
            <input className="input" value={form.maintenance.backups_dir}
              onChange={e => setForm(f => f ? ({ ...f, maintenance: { ...f.maintenance, backups_dir: e.target.value } }) : f)} />
          </Field>
          <Field label="Check retention days" hint="0 keeps all history forever">
            <input className="input" type="number" min={0} value={form.maintenance.check_retention_days}
              onChange={e => setForm(f => f ? ({ ...f, maintenance: { ...f.maintenance, check_retention_days: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Audit retention days" hint="0 keeps audit history forever">
            <input className="input" type="number" min={0} value={form.maintenance.audit_retention_days}
              onChange={e => setForm(f => f ? ({ ...f, maintenance: { ...f.maintenance, audit_retention_days: Number(e.target.value) } }) : f)} />
          </Field>
          <Field label="Retention sweep interval" hint="e.g. 24h">
            <input className="input" value={form.maintenance.retention_sweep_interval}
              onChange={e => setForm(f => f ? ({ ...f, maintenance: { ...f.maintenance, retention_sweep_interval: e.target.value } }) : f)} />
          </Field>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[{ title: 'Health', payload: health }, { title: 'Readiness', payload: readiness }].map(item => (
            <div key={item.title} className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-2">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-semibold text-white">{item.title}</div>
                <span className={`rounded-full px-2 py-1 text-[11px] ${item.payload?.status === 'ok' ? 'bg-emerald-500/10 text-emerald-300' : 'bg-amber-500/10 text-amber-300'}`}>
                  {item.payload?.status ?? 'unknown'}
                </span>
              </div>
              <div className="text-xs text-slate-400">Database: <span className="text-slate-200">{item.payload?.database ?? 'loading'}</span></div>
              <div className="text-xs text-slate-400">Scheduler: <span className="text-slate-200">{item.payload?.scheduler.started ? 'running' : 'not ready'}</span></div>
              <div className="text-xs text-slate-500">In flight checks: {item.payload?.scheduler.in_flight ?? 0}</div>
            </div>
          ))}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-[0.95fr_1.05fr] gap-6">
          <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
            <div className="text-sm font-semibold text-white">Run maintenance</div>
            <div className="flex flex-wrap items-end gap-3">
              <div className="min-w-[180px] flex-1">
                <Field label="Manual prune horizon">
                  <input className="input" type="number" min={1} value={pruneDays} onChange={e => setPruneDays(Number(e.target.value))} />
                </Field>
              </div>
              <button className="btn-primary" onClick={() => backupMutation.mutate()} disabled={backupMutation.isPending}>
                {backupMutation.isPending ? 'Creating backup...' : 'Create backup'}
              </button>
              <button className="btn-ghost border border-slate-700" onClick={() => pruneMutation.mutate(pruneDays)} disabled={pruneMutation.isPending}>
                {pruneMutation.isPending ? 'Pruning...' : 'Prune history'}
              </button>
            </div>
            <div className="text-xs text-slate-400">
              Runtime restore is intentionally not exposed in the UI. Use the CLI <code>-restore</code> flow during maintenance windows.
            </div>
          </div>

          <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
            <div className="text-sm font-semibold text-white">Recent backups</div>
            {backupFiles.length === 0 ? (
              <div className="text-sm text-slate-400">No backup files found yet.</div>
            ) : (
              <div className="space-y-2">
                {backupFiles.slice(0, 6).map((file: BackupFile) => (
                  <div key={file.path} className="rounded-lg border border-slate-800 bg-slate-950/60 px-3 py-2 text-xs text-slate-300">
                    <div className="font-medium text-white">{file.name}</div>
                    <div className="mt-1 break-all text-slate-500">{file.path}</div>
                    <div className="mt-1 text-slate-500">{new Date(file.modified_at).toLocaleString()} | {(file.size_bytes / 1024 / 1024).toFixed(2)} MB</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </Section>

      <Section title="Audit Log" icon={Activity}>
        <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden">
          {auditLogs.length === 0 ? (
            <div className="p-4 text-sm text-slate-400">No audit events recorded yet.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead className="bg-slate-950/70 text-slate-400">
                  <tr>
                    <th className="px-4 py-3 font-medium">When</th>
                    <th className="px-4 py-3 font-medium">Actor</th>
                    <th className="px-4 py-3 font-medium">Action</th>
                    <th className="px-4 py-3 font-medium">Summary</th>
                    <th className="px-4 py-3 font-medium">Request ID</th>
                  </tr>
                </thead>
                <tbody>
                  {auditLogs.slice(0, 50).map((entry: AuditLog) => (
                    <tr key={entry.id} className="border-t border-slate-800 text-slate-300">
                      <td className="px-4 py-3 align-top text-xs text-slate-400">{new Date(entry.created_at).toLocaleString()}</td>
                      <td className="px-4 py-3 align-top">
                        <div className="font-medium text-white">{entry.actor_username || 'anonymous'}</div>
                        <div className="text-xs text-slate-500">{entry.actor_role} via {entry.actor_source}</div>
                      </td>
                      <td className="px-4 py-3 align-top text-xs uppercase tracking-wide text-slate-400">{entry.resource}.{entry.action}</td>
                      <td className="px-4 py-3 align-top">
                        <div>{entry.summary}</div>
                        {entry.details && Object.keys(entry.details).length > 0 && (
                          <pre className="mt-2 overflow-auto rounded bg-slate-950/70 p-2 text-[11px] text-slate-400">{JSON.stringify(entry.details, null, 2)}</pre>
                        )}
                      </td>
                      <td className="px-4 py-3 align-top text-xs text-slate-500">{entry.request_id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </Section>

      <Section title="Local Users & Roles" icon={Users}>
        <div className="rounded-xl border border-slate-800 bg-slate-900/40 px-4 py-3 text-xs text-slate-400">
          Keep at least one enabled <code>admin</code> account at all times. The backend blocks removing the last enabled admin, and the UI highlights the current draft before save.
          <div className="mt-2 text-slate-300">Enabled admins in current draft: {enabledAdminCount}</div>
        </div>
        <div className="grid grid-cols-1 lg:grid-cols-[1.1fr_0.9fr] gap-6">
          <div className="space-y-3">
            {editableUsers.length === 0 ? (
              <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 text-sm text-slate-400">No local users yet. You can create viewer, editor, or admin accounts below.</div>
            ) : editableUsers.map(user => (
              <div key={user.id} className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
                <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
                  <Field label="Username">
                    <input
                      className="input"
                      value={user.username}
                      onChange={e => setEditableUsers(list => list.map(item => item.id === user.id ? { ...item, username: e.target.value } : item))}
                    />
                  </Field>
                  <Field label="Role">
                    <select
                      className="select"
                      value={user.role}
                      onChange={e => setEditableUsers(list => list.map(item => item.id === user.id ? { ...item, role: e.target.value as UserAccount['role'] } : item))}
                    >
                      <option value="viewer">viewer</option>
                      <option value="editor">editor</option>
                      <option value="admin">admin</option>
                    </select>
                  </Field>
                  <Field label="Enabled">
                    <BoolSelect
                      value={user.enabled}
                      onChange={value => setEditableUsers(list => list.map(item => item.id === user.id ? { ...item, enabled: value } : item))}
                    />
                  </Field>
                  <Field label="New password" hint="Leave blank to keep the current password">
                    <input
                      className="input"
                      type="password"
                      value={user.password ?? ''}
                      onChange={e => setEditableUsers(list => list.map(item => item.id === user.id ? { ...item, password: e.target.value } : item))}
                    />
                  </Field>
                </div>
                <div className="flex items-center justify-between gap-3 flex-wrap text-xs text-slate-500">
                  <div>
                    Created {new Date(user.created_at).toLocaleString()}
                    {user.last_login_at && ` | Last login ${new Date(user.last_login_at).toLocaleString()}`}
                  </div>
                  <div className="flex items-center gap-2">
                    <button
                      className="btn-ghost border border-slate-700"
                      onClick={() => updateUserMutation.mutate({
                        id: user.id,
                        payload: {
                          username: user.username,
                          role: user.role,
                          enabled: user.enabled,
                          password: user.password?.trim() || undefined,
                        },
                      })}
                      disabled={wouldLeaveNoEnabledAdmins(users, user)}
                    >
                      Save user
                    </button>
                    <button
                      className="btn-danger"
                      onClick={() => deleteUserMutation.mutate(user.id)}
                      disabled={me?.username === user.username}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              </div>
            ))}
          </div>

          <div className="rounded-xl border border-blue-500/15 bg-blue-500/5 p-4 space-y-3">
            <div className="text-sm font-semibold text-white">Create local user</div>
            <Field label="Username">
              <input className="input" value={newUser.username} onChange={e => setNewUser(user => ({ ...user, username: e.target.value }))} />
            </Field>
            <Field label="Role">
              <select className="select" value={newUser.role} onChange={e => setNewUser(user => ({ ...user, role: e.target.value as UserWritePayload['role'] }))}>
                <option value="viewer">viewer</option>
                <option value="editor">editor</option>
                <option value="admin">admin</option>
              </select>
            </Field>
            <Field label="Enabled">
              <BoolSelect value={newUser.enabled ?? true} onChange={value => setNewUser(user => ({ ...user, enabled: value }))} />
            </Field>
            <Field label="Password">
              <input className="input" type="password" value={newUser.password ?? ''} onChange={e => setNewUser(user => ({ ...user, password: e.target.value }))} />
            </Field>
            <button
              className="btn-primary w-full justify-center"
              disabled={createUserMutation.isPending}
              onClick={() => createUserMutation.mutate(newUser)}
            >
              {createUserMutation.isPending ? 'Creating...' : 'Create user'}
            </button>
            <div className="text-xs text-slate-400">
              <div>`viewer` can browse the UI.</div>
              <div>`editor` can add, edit, reorder, and trigger checks.</div>
              <div>`admin` can manage settings and users.</div>
            </div>
          </div>
        </div>
      </Section>
    </div>
  )
}

function matchesStatusPolicy(a: AppConfig['status_policy'], b: AppConfig['status_policy']): boolean {
  return a.badge_on_invalid_chain === b.badge_on_invalid_chain &&
    a.badge_on_self_signed === b.badge_on_self_signed &&
    a.badge_on_http_probe_error === b.badge_on_http_probe_error &&
    a.badge_on_http_client_error === b.badge_on_http_client_error &&
    a.badge_on_cipher_warning === b.badge_on_cipher_warning &&
    a.badge_on_ocsp_unknown === b.badge_on_ocsp_unknown &&
    a.badge_on_crl_unknown === b.badge_on_crl_unknown &&
    a.badge_on_caa_missing === b.badge_on_caa_missing &&
    a.badge_on_domain_lookup_error === b.badge_on_domain_lookup_error
}

function wouldLeaveNoEnabledAdmins(users: UserAccount[], draft: EditableUser): boolean {
  const resultingAdmins = users.filter(user => {
    const effective = user.id === draft.id ? draft : user
    return effective.enabled && effective.role === 'admin'
  }).length
  return resultingAdmins === 0
}
