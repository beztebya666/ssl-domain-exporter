import { useEffect, useMemo, useState } from 'react'
import { BellRing, Mail, Send, Webhook } from 'lucide-react'
import ModalShell from './ModalShell'
import SecretInput from './SecretInput'
import type {
  AdHocNotificationRequest,
  AdHocNotificationResult,
  NotificationChannel,
} from '../types'

type Props = {
  open: boolean
  domainName: string
  busy: boolean
  results: AdHocNotificationResult[]
  onClose: () => void
  onSubmit: (payload: AdHocNotificationRequest) => void
}

type ChannelSelection = Record<NotificationChannel, boolean>

const defaultSelection: ChannelSelection = {
  email: false,
  webhook: false,
  telegram: false,
}

export default function AdHocNotificationModal({
  open,
  domainName,
  busy,
  results,
  onClose,
  onSubmit,
}: Props) {
  const [subject, setSubject] = useState('')
  const [message, setMessage] = useState('')
  const [emailTo, setEmailTo] = useState('')
  const [webhookURL, setWebhookURL] = useState('')
  const [telegramBotToken, setTelegramBotToken] = useState('')
  const [telegramChatID, setTelegramChatID] = useState('')
  const [selectedChannels, setSelectedChannels] = useState<ChannelSelection>(defaultSelection)

  useEffect(() => {
    if (!open) {
      setSubject('')
      setMessage('')
      setEmailTo('')
      setWebhookURL('')
      setTelegramBotToken('')
      setTelegramChatID('')
      setSelectedChannels(defaultSelection)
    }
  }, [open])

  const enabledChannels = useMemo(
    () => (Object.entries(selectedChannels)
      .filter(([, enabled]) => enabled)
      .map(([channel]) => channel)) as NotificationChannel[],
    [selectedChannels],
  )

  if (!open) return null

  return (
    <ModalShell
      onClose={busy ? undefined : onClose}
      panelClassName="max-w-3xl"
      title={(
        <div className="flex items-center gap-2">
          <BellRing size={16} className="text-blue-300" />
          Send ad-hoc notification
        </div>
      )}
      description={`Notify operators right now for ${domainName}. Leave channels empty to auto-detect configured delivery targets.`}
      footer={(
        <>
          <button type="button" className="btn-ghost flex-1 border border-slate-700" onClick={onClose} disabled={busy}>
            Cancel
          </button>
          <button
            type="button"
            className="btn-primary flex-1 justify-center"
            onClick={() => onSubmit({
              channels: enabledChannels,
              subject: subject.trim() || undefined,
              message: message.trim() || undefined,
              email_to: splitList(emailTo),
              webhook_url: webhookURL.trim() || undefined,
              telegram_bot_token: telegramBotToken.trim() || undefined,
              telegram_chat_id: telegramChatID.trim() || undefined,
            })}
            disabled={busy}
          >
            {busy ? 'Sending...' : 'Send notification'}
          </button>
        </>
      )}
    >
      <div className="space-y-5">
        <div>
          <div className="text-xs font-semibold uppercase tracking-wide text-slate-500">Channels</div>
          <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
            <ChannelToggle
              icon={Mail}
              label="Email"
              description="Use saved SMTP settings or override recipients below."
              active={selectedChannels.email}
              onClick={() => setSelectedChannels(current => ({ ...current, email: !current.email }))}
            />
            <ChannelToggle
              icon={Webhook}
              label="Webhook"
              description="POST a one-off payload to automation or a relay."
              active={selectedChannels.webhook}
              onClick={() => setSelectedChannels(current => ({ ...current, webhook: !current.webhook }))}
            />
            <ChannelToggle
              icon={Send}
              label="Telegram"
              description="Send directly to an incident or on-call chat."
              active={selectedChannels.telegram}
              onClick={() => setSelectedChannels(current => ({ ...current, telegram: !current.telegram }))}
            />
          </div>
          <p className="mt-2 text-xs text-slate-500">
            {enabledChannels.length === 0
              ? 'No channels selected: the backend will auto-pick configured email, webhook, and Telegram routes.'
              : `Selected channels: ${enabledChannels.join(', ')}`}
          </p>
        </div>

        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label className="label">Subject</label>
            <input
              className="input"
              value={subject}
              onChange={event => setSubject(event.target.value)}
              placeholder="Leave blank to use the generated alert subject"
            />
          </div>
          <div>
            <label className="label">Email recipients override</label>
            <input
              className="input"
              value={emailTo}
              onChange={event => setEmailTo(event.target.value)}
              placeholder="alice@example.com, team@example.com"
            />
          </div>
        </div>

        <div>
          <label className="label">Message</label>
          <textarea
            className="input h-32 resize-y"
            value={message}
            onChange={event => setMessage(event.target.value)}
            placeholder="Leave blank to send the generated domain status summary"
          />
        </div>

        <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4">
          <div className="text-sm font-semibold text-white">Optional one-off overrides</div>
          <div className="mt-3 grid grid-cols-1 gap-4 md:grid-cols-2">
            <div>
              <label className="label">Webhook URL override</label>
              <input
                className="input"
                value={webhookURL}
                onChange={event => setWebhookURL(event.target.value)}
                placeholder="https://hooks.example.com/..."
              />
            </div>
            <div>
              <label className="label">Telegram chat ID override</label>
              <input
                className="input"
                value={telegramChatID}
                onChange={event => setTelegramChatID(event.target.value)}
                placeholder="-1001234567890"
              />
            </div>
            <div className="md:col-span-2">
              <label className="label">Telegram bot token override</label>
              <SecretInput
                value={telegramBotToken}
                ariaLabel="Telegram bot token override"
                onChange={setTelegramBotToken}
              />
            </div>
          </div>
        </div>

        {results.length > 0 && (
          <div className="rounded-xl border border-slate-800 bg-slate-950/60 p-4">
            <div className="text-sm font-semibold text-white">Latest delivery results</div>
            <div className="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
              {results.map(result => (
                <div
                  key={result.channel}
                  className={`rounded-xl border px-4 py-3 text-sm ${
                    result.success
                      ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-200'
                      : 'border-rose-500/20 bg-rose-500/10 text-rose-200'
                  }`}
                >
                  <div className="font-medium text-white">{channelLabel(result.channel)}</div>
                  <div className="mt-1">{result.success ? 'Delivered successfully.' : result.error || 'Delivery failed.'}</div>
                  {result.recipients && result.recipients.length > 0 && (
                    <div className="mt-2 text-xs text-slate-300">
                      Recipients: {result.recipients.join(', ')}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </ModalShell>
  )
}

function ChannelToggle({
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
      aria-pressed={active}
    >
      <div className="flex items-start gap-3">
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

function splitList(raw: string): string[] | undefined {
  const values = raw
    .split(',')
    .map(value => value.trim())
    .filter(Boolean)
  return values.length > 0 ? values : undefined
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
