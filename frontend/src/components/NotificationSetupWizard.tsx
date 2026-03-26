import { useMemo, useState } from 'react'
import { Bell, Mail, Send, Webhook } from 'lucide-react'
import type { Dispatch, SetStateAction } from 'react'
import SecretInput from './SecretInput'
import type { AppConfig, NotificationChannel, NotificationDeliveryStatus, NotificationTestResult } from '../types'

type Props = {
  form: AppConfig
  setForm: Dispatch<SetStateAction<AppConfig | null>>
  notificationStatus: NotificationDeliveryStatus[]
  notificationTestResults: NotificationTestResult[]
  testing: boolean
  onSendTest: (channel: NotificationChannel) => void
}

const wizardSteps = ['Channel', 'Connection', 'Routing', 'Test'] as const

export default function NotificationSetupWizard({
  form,
  setForm,
  notificationStatus,
  notificationTestResults,
  testing,
  onSendTest,
}: Props) {
  const [channel, setChannel] = useState<NotificationChannel>('email')
  const [stepIndex, setStepIndex] = useState(0)

  const currentStatus = useMemo(
    () => notificationStatus.find(item => item.channel === channel),
    [channel, notificationStatus],
  )
  const currentTest = useMemo(
    () => notificationTestResults.find(item => item.channel === channel),
    [channel, notificationTestResults],
  )
  const currentChannelEnabled = channelEnabledFromForm(form, channel)
  const emailRecipients = useMemo(() => (form.notifications.email.to ?? []).join(', '), [form.notifications.email.to])

  const setChannelEnabled = (enabled: boolean) => {
    setForm(current => {
      if (!current) return current
      return {
        ...current,
        notifications: {
          ...current.notifications,
          [channel]: {
            ...current.notifications[channel],
            enabled,
          },
        },
      }
    })
  }

  const setChannelPatch = (patch: Record<string, unknown>) => {
    setForm(current => {
      if (!current) return current
      return {
        ...current,
        notifications: {
          ...current.notifications,
          [channel]: {
            ...current.notifications[channel],
            ...patch,
          },
        },
      }
    })
  }

  return (
    <div className="rounded-2xl border border-blue-500/15 bg-blue-500/5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 text-sm font-semibold text-white">
            <Bell size={16} className="text-blue-400" />
            Notification setup wizard
          </div>
          <p className="mt-1 max-w-2xl text-xs text-slate-400">
            Configure delivery in a guided flow, then run a live test against the selected channel before saving.
          </p>
        </div>
        <div className="min-w-[180px]">
          <label className="label" htmlFor="notifications-timeout">Timeout</label>
          <input
            id="notifications-timeout"
            className="input"
            value={form.notifications.timeout}
            onChange={e => setForm(current => current ? ({
              ...current,
              notifications: { ...current.notifications, timeout: e.target.value },
            }) : current)}
          />
        </div>
      </div>

      <div className="mt-5 grid grid-cols-1 gap-3 md:grid-cols-3">
        <ChannelCard
          icon={Mail}
          label="Email"
          description="SMTP delivery for enterprise inboxes and ticketing relays."
          active={channel === 'email'}
          onClick={() => setChannel('email')}
        />
        <ChannelCard
          icon={Webhook}
          label="Webhook"
          description="Push alert payloads to automation, SOAR, or chat relays."
          active={channel === 'webhook'}
          onClick={() => setChannel('webhook')}
        />
        <ChannelCard
          icon={Send}
          label="Telegram"
          description="Fast delivery to operational chat rooms and on-call groups."
          active={channel === 'telegram'}
          onClick={() => setChannel('telegram')}
        />
      </div>

      <div className="mt-5 flex flex-wrap gap-2">
        {wizardSteps.map((step, index) => (
          <button
            key={step}
            type="button"
            className={`inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs transition-colors ${
              stepIndex === index
                ? 'border-blue-500/30 bg-blue-500/10 text-blue-300'
                : 'border-slate-700 bg-slate-900/40 text-slate-400 hover:border-slate-600 hover:text-slate-200'
            }`}
            onClick={() => setStepIndex(index)}
          >
            <span className={`flex h-5 w-5 items-center justify-center rounded-full text-[11px] ${stepIndex === index ? 'bg-blue-500 text-white' : 'bg-slate-800 text-slate-300'}`}>
              {index + 1}
            </span>
            {step}
          </button>
        ))}
      </div>

      <div className="mt-5 rounded-2xl border border-slate-800 bg-slate-950/40 p-4">
        {stepIndex === 0 && (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Field label="Global notifications">
              <select
                className="select"
                value={form.features.notifications ? 'true' : 'false'}
                onChange={e => setForm(current => current ? ({
                  ...current,
                  features: { ...current.features, notifications: e.target.value === 'true' },
                }) : current)}
              >
                <option value="true">Enabled</option>
                <option value="false">Disabled</option>
              </select>
            </Field>
            <Field label={`${channelLabel(channel)} channel`}>
              <select className="select" value={channelValue(currentChannelEnabled)} onChange={e => setChannelEnabled(e.target.value === 'true')}>
                <option value="true">Enabled</option>
                <option value="false">Disabled</option>
              </select>
            </Field>
          </div>
        )}

        {stepIndex === 1 && (
          <>
            {channel === 'email' && (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <Field label="SMTP host">
                  <input className="input" value={form.notifications.email.host} onChange={e => setChannelPatch({ host: e.target.value })} />
                </Field>
                <Field label="SMTP port">
                  <input className="input" type="number" value={form.notifications.email.port} onChange={e => setChannelPatch({ port: Number(e.target.value) || 0 })} />
                </Field>
                <Field label="SMTP mode">
                  <select className="select" value={form.notifications.email.mode} onChange={e => setChannelPatch({ mode: e.target.value })}>
                    <option value="starttls">starttls</option>
                    <option value="tls">tls</option>
                    <option value="none">none</option>
                  </select>
                </Field>
                <Field label="SMTP username">
                  <input className="input" value={form.notifications.email.username} onChange={e => setChannelPatch({ username: e.target.value })} />
                </Field>
                <Field label="SMTP password">
                  <SecretInput
                    value={form.notifications.email.password}
                    ariaLabel="SMTP password"
                    onChange={value => setChannelPatch({ password: value })}
                  />
                </Field>
                <Field label="From address">
                  <input className="input" value={form.notifications.email.from} onChange={e => setChannelPatch({ from: e.target.value })} />
                </Field>
              </div>
            )}

            {channel === 'webhook' && (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <Field label="Webhook URL">
                  <SecretInput
                    type="text"
                    value={form.notifications.webhook.url}
                    ariaLabel="Webhook URL"
                    onChange={value => setChannelPatch({ url: value })}
                  />
                </Field>
                <Field label="Delivery mode">
                  <div className="rounded-xl border border-slate-800 bg-slate-900/40 px-4 py-3 text-sm text-slate-300">
                    JSON payload delivery using the global timeout. Use the test step to validate routing before enabling alerts.
                  </div>
                </Field>
              </div>
            )}

            {channel === 'telegram' && (
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <Field label="Bot token">
                  <SecretInput
                    value={form.notifications.telegram.bot_token}
                    ariaLabel="Telegram bot token"
                    onChange={value => setChannelPatch({ bot_token: value })}
                  />
                </Field>
                <Field label="Chat ID">
                  <input className="input" value={form.notifications.telegram.chat_id} onChange={e => setChannelPatch({ chat_id: e.target.value })} />
                </Field>
              </div>
            )}
          </>
        )}

        {stepIndex === 2 && (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            {channel === 'email' && (
              <>
                <Field label="Recipients" hint="Comma-separated email addresses">
                  <input className="input" value={emailRecipients} onChange={e => setChannelPatch({ to: e.target.value.split(',').map(v => v.trim()).filter(Boolean) })} />
                </Field>
                <Field label="Subject prefix">
                  <input className="input" value={form.notifications.email.subject_prefix} onChange={e => setChannelPatch({ subject_prefix: e.target.value })} />
                </Field>
                <Field label="Skip TLS verification">
                  <select className="select" value={channelValue(form.notifications.email.insecure_skip_verify)} onChange={e => setChannelPatch({ insecure_skip_verify: e.target.value === 'true' })}>
                    <option value="false">Disabled</option>
                    <option value="true">Enabled</option>
                  </select>
                </Field>
              </>
            )}
            <Field label={`${channelLabel(channel)} on critical`}>
              <select className="select" value={channelValue(currentTriggers(form, channel).onCritical)} onChange={e => setTrigger(setChannelPatch, channel, 'onCritical', e.target.value === 'true')}>
                <option value="true">Enabled</option>
                <option value="false">Disabled</option>
              </select>
            </Field>
            <Field label={`${channelLabel(channel)} on warning`}>
              <select className="select" value={channelValue(currentTriggers(form, channel).onWarning)} onChange={e => setTrigger(setChannelPatch, channel, 'onWarning', e.target.value === 'true')}>
                <option value="false">Disabled</option>
                <option value="true">Enabled</option>
              </select>
            </Field>
          </div>
        )}

        {stepIndex === 3 && (
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 text-sm">
                <div className="flex items-center justify-between gap-3">
                  <div className="font-medium text-white">{channelLabel(channel)} status</div>
                  <span className={`rounded-full px-2 py-1 text-[11px] ${currentChannelEnabled ? 'bg-emerald-500/10 text-emerald-300' : 'bg-slate-700 text-slate-300'}`}>
                    {currentChannelEnabled ? 'enabled' : 'disabled'}
                  </span>
                </div>
                <div className="mt-3 space-y-1 text-xs text-slate-400">
                  <div>Last attempt: {currentStatus?.last_attempt_at ? new Date(currentStatus.last_attempt_at).toLocaleString() : 'never'}</div>
                  <div>Last success: {currentStatus?.last_success_at ? new Date(currentStatus.last_success_at).toLocaleString() : 'never'}</div>
                  <div className={currentStatus?.last_error ? 'text-rose-300' : 'text-slate-500'}>
                    {currentStatus?.last_error ? `Last error: ${currentStatus.last_error}` : 'No recent delivery errors'}
                  </div>
                </div>
              </div>

              <div className="rounded-xl border border-blue-500/15 bg-blue-500/5 p-4 text-sm">
                <div className="font-medium text-white">Live test</div>
                <div className="mt-2 text-xs text-slate-400">
                  Send a real test notification through the selected channel with the current configuration values.
                </div>
                <button className="btn-primary mt-4" onClick={() => onSendTest(channel)} disabled={testing}>
                  {testing ? 'Sending test...' : `Send ${channelLabel(channel).toLowerCase()} test`}
                </button>
                {currentTest && (
                  <div className={`mt-3 text-xs ${currentTest.success ? 'text-emerald-300' : 'text-rose-300'}`}>
                    {currentTest.enabled
                      ? (currentTest.success ? 'Latest test succeeded.' : currentTest.error || 'Latest test failed.')
                      : 'Channel is disabled.'}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

function ChannelCard({
  icon: Icon,
  label,
  description,
  active,
  onClick,
}: {
  icon: typeof Mail
  label: string
  description: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className={`rounded-2xl border p-4 text-left transition-colors ${
        active ? 'border-blue-500/30 bg-blue-500/10' : 'border-slate-800 bg-slate-950/40 hover:border-slate-700'
      }`}
      onClick={onClick}
    >
      <div className="flex items-center gap-3">
        <div className={`rounded-xl p-2 ${active ? 'bg-blue-500/15 text-blue-300' : 'bg-slate-800 text-slate-300'}`}>
          <Icon size={16} />
        </div>
        <div>
          <div className="text-sm font-semibold text-white">{label}</div>
          <div className="mt-1 text-xs text-slate-400">{description}</div>
        </div>
      </div>
    </button>
  )
}

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div>
      <label className="label">{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  )
}

function channelLabel(channel: NotificationChannel): string {
  switch (channel) {
    case 'email':
      return 'Email'
    case 'webhook':
      return 'Webhook'
    default:
      return 'Telegram'
  }
}

function channelValue(value: boolean | undefined): 'true' | 'false' {
  return value ? 'true' : 'false'
}

function channelEnabledFromForm(form: AppConfig, channel: NotificationChannel): boolean {
  switch (channel) {
    case 'email':
      return form.notifications.email.enabled
    case 'webhook':
      return form.notifications.webhook.enabled
    default:
      return form.notifications.telegram.enabled
  }
}

function currentTriggers(form: AppConfig, channel: NotificationChannel): { onCritical: boolean; onWarning: boolean } {
  switch (channel) {
    case 'email':
      return { onCritical: form.notifications.email.on_critical, onWarning: form.notifications.email.on_warning }
    case 'webhook':
      return { onCritical: form.notifications.webhook.on_critical, onWarning: form.notifications.webhook.on_warning }
    default:
      return { onCritical: form.notifications.telegram.on_critical, onWarning: form.notifications.telegram.on_warning }
  }
}

function setTrigger(
  setChannelPatch: (patch: Record<string, unknown>) => void,
  channel: NotificationChannel,
  key: 'onCritical' | 'onWarning',
  value: boolean,
) {
  const mapping = {
    email: { onCritical: 'on_critical', onWarning: 'on_warning' },
    webhook: { onCritical: 'on_critical', onWarning: 'on_warning' },
    telegram: { onCritical: 'on_critical', onWarning: 'on_warning' },
  } satisfies Record<NotificationChannel, Record<'onCritical' | 'onWarning', string>>

  setChannelPatch({ [mapping[channel][key]]: value })
}
