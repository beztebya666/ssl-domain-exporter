import { lazy, Suspense, useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  BellRing,
  ArrowLeft, RefreshCw, Shield, Globe, Link2, Clock,
  CheckCircle, XCircle, AlertTriangle, ExternalLink, ChevronDown, ChevronUp, Pencil, Check, X, Server
} from 'lucide-react'
import { fetchDomain, fetchHistory, fetchHistoryPage, triggerCheck, updateDomain, fetchFolders, fetchCustomFields, sendAdHocNotification } from '../api/client'
import AdHocNotificationModal from '../components/AdHocNotificationModal'
import StatusBadge from '../components/StatusBadge'
import TagEditor from '../components/TagEditor'
import MetadataEditor from '../components/MetadataEditor'
import CustomFieldInputs from '../components/CustomFieldInputs'
import CollapsiblePanel from '../components/CollapsiblePanel'
import EmptyState from '../components/EmptyState'
import Pagination from '../components/Pagination'
import { DetailSkeleton, Skeleton } from '../components/Skeleton'
import { useToast } from '../components/ToastProvider'
import { format, formatDistanceToNow } from 'date-fns'
import type { AdHocNotificationRequest, AdHocNotificationResult, AuthMe, BootstrapConfig, ChainCert } from '../types'
import { metadataSearchText } from '../lib/domainFields'
import { mergeSchemaAndExtraMetadata, splitMetadataBySchema, visibleMetadataSummary } from '../lib/customFields'
import InventorySourceEditor from '../components/InventorySourceEditor'
import { buildSourceRef, buildSourceWritePayload, createInventorySourceDraft, formatSourceSummary, isManualSource, sourceTypeBadge, sourceTypeLabel } from '../lib/domainSources'
import { getErrorMessage } from '../lib/utils'

const ExpiryHistoryChart = lazy(() => import('../components/ExpiryHistoryChart'))

function InfoRow({ label, value, mono = false }: { label: string; value?: string | number | null; mono?: boolean }) {
  if (!value && value !== 0) return null
  return (
    <div className="flex justify-between py-2 border-b border-gray-800 last:border-0">
      <span className="text-xs text-gray-500">{label}</span>
      <span className={`text-xs text-gray-200 text-right max-w-[65%] break-words whitespace-normal ${mono ? 'font-mono' : ''}`}>{value}</span>
    </div>
  )
}

function ChainCard({ cert, index }: { cert: ChainCert; index: number }) {
  const [open, setOpen] = useState(index === 0)
  const isExpired = new Date(cert.valid_to) < new Date()
  return (
    <div className="border border-gray-800 rounded-lg overflow-hidden">
      <button
        className="w-full flex items-center justify-between p-3 hover:bg-gray-800 transition-colors text-left"
        onClick={() => setOpen(!open)}
      >
        <div className="flex items-center gap-3">
          <span className="text-xs font-mono text-gray-600 w-5">{index + 1}</span>
          {cert.is_ca ? (
            <span className="text-xs text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded">CA</span>
          ) : (
            <span className="text-xs text-green-400 bg-green-500/10 px-2 py-0.5 rounded">Leaf</span>
          )}
          {cert.is_self_signed && (
            <span className="text-xs text-yellow-400 bg-yellow-500/10 px-2 py-0.5 rounded">Self-signed</span>
          )}
          <span className="text-sm font-medium text-gray-200 truncate">{cert.subject}</span>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          {isExpired ? (
            <XCircle size={14} className="text-red-400" />
          ) : (
            <CheckCircle size={14} className="text-green-400" />
          )}
          {open ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
        </div>
      </button>
      {open && (
        <div className="p-3 border-t border-gray-800 bg-gray-900/50 space-y-0">
          <InfoRow label="Subject" value={cert.subject} />
          <InfoRow label="Issuer" value={cert.issuer} />
          <InfoRow label="Valid From" value={format(new Date(cert.valid_from), 'yyyy-MM-dd HH:mm')} />
          <InfoRow label="Valid Until" value={format(new Date(cert.valid_to), 'yyyy-MM-dd HH:mm')} />
          <InfoRow label="Is CA" value={cert.is_ca ? 'Yes' : 'No'} />
          <InfoRow label="Self-signed" value={cert.is_self_signed ? 'Yes' : 'No'} />
        </div>
      )}
    </div>
  )
}

type DomainDetailProps = {
  me?: AuthMe
  bootstrap?: BootstrapConfig
}

export default function DomainDetail({ me, bootstrap }: DomainDetailProps) {
  const { id } = useParams<{ id: string }>()
  const domainId = Number(id)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { showToast } = useToast()
  const [checking, setChecking] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editSourceDraft, setEditSourceDraft] = useState(() => createInventorySourceDraft())
  const [editTags, setEditTags] = useState<string[]>([])
  const [editMetadata, setEditMetadata] = useState<Record<string, string>>({})
  const [editEnabled, setEditEnabled] = useState(true)
  const [editInterval, setEditInterval] = useState(21600)
  const [editPort, setEditPort] = useState(443)
  const [editFolderValue, setEditFolderValue] = useState('')
  const [editCustomCAPEM, setEditCustomCAPEM] = useState('')
  const [editCheckMode, setEditCheckMode] = useState('full')
  const [editDnsServers, setEditDnsServers] = useState('')
  const [historyPage, setHistoryPage] = useState(1)
  const [notifyOpen, setNotifyOpen] = useState(false)
  const [notificationResults, setNotificationResults] = useState<AdHocNotificationResult[]>([])
  const canEdit = me?.can_edit ?? false
  const notificationsEnabled = bootstrap?.features.notifications ?? true

  const { data: domain, isLoading } = useQuery({
    queryKey: ['domain', domainId],
    queryFn: () => fetchDomain(domainId),
    enabled: !!domainId,
  })

  const { data: historyPreview = [] } = useQuery({
    queryKey: ['history-preview', domainId],
    queryFn: () => fetchHistory(domainId, 60),
    enabled: !!domainId,
  })
  const { data: historyPageData } = useQuery({
    queryKey: ['history-page', domainId, historyPage],
    queryFn: () => fetchHistoryPage(domainId, historyPage, 20),
    enabled: !!domainId,
    placeholderData: previous => previous,
  })
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })
  const { data: customFields = [] } = useQuery({ queryKey: ['custom-fields'], queryFn: () => fetchCustomFields(false) })

  useEffect(() => {
    setHistoryPage(1)
  }, [domainId])

  const updateMutation = useMutation({
    mutationFn: (data: Parameters<typeof updateDomain>[1]) => updateDomain(domainId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domain', domainId] })
      qc.invalidateQueries({ queryKey: ['domains'] })
      qc.invalidateQueries({ queryKey: ['domains-page'] })
      qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
      qc.invalidateQueries({ queryKey: ['tags'] })
      setEditing(false)
      showToast({ tone: 'success', text: 'Domain updated.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to update domain.') }),
  })

  const notifyMutation = useMutation({
    mutationFn: (payload: AdHocNotificationRequest) => sendAdHocNotification(domainId, payload),
    onSuccess: (response) => {
      setNotificationResults(response.results ?? [])
      qc.invalidateQueries({ queryKey: ['audit-logs'] })
      const failures = (response.results ?? []).filter(result => !result.success)
      showToast({
        tone: failures.length > 0 ? 'error' : 'success',
        text: failures.length > 0
          ? 'Ad-hoc notification completed with channel failures. Review the delivery results.'
          : 'Ad-hoc notification sent successfully.',
      })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to send ad-hoc notification.') }),
  })

  const startEdit = () => {
    if (!domain) return
    setEditSourceDraft(createInventorySourceDraft(domain))
    setEditTags(domain.tags)
    setEditMetadata(domain.metadata ?? {})
    setEditEnabled(domain.enabled)
    setEditInterval(domain.check_interval)
    setEditPort(domain.port || 443)
    setEditFolderValue(domain.folder_id ? String(domain.folder_id) : '')
    setEditCustomCAPEM(domain.custom_ca_pem ?? '')
    setEditCheckMode(domain.check_mode || 'full')
    setEditDnsServers(domain.dns_servers || '')
    setEditing(true)
  }

  const saveEdit = () => {
    const sourcePayload = buildSourceWritePayload(editSourceDraft)
    if (sourcePayload.source_type === 'manual' && !sourcePayload.name?.trim()) {
      showToast({ tone: 'error', text: 'Domain / host is required for manual endpoints.' })
      return
    }

    updateMutation.mutate({
      name: sourcePayload.name,
      source_type: sourcePayload.source_type,
      source_ref: sourcePayload.source_ref,
      tags: editTags,
      metadata: editMetadata,
      enabled: editEnabled,
      check_interval: editInterval,
      folder_id: editFolderValue ? Number(editFolderValue) : null,
      ...(sourcePayload.source_type === 'manual'
        ? {
            port: editPort,
            custom_ca_pem: editCustomCAPEM,
            check_mode: editCheckMode,
            dns_servers: editDnsServers,
          }
        : {}),
    })
  }

  const handleCheck = async () => {
    setChecking(true)
    try {
      await triggerCheck(domainId)
      qc.invalidateQueries({ queryKey: ['domain', domainId] })
      qc.invalidateQueries({ queryKey: ['history-preview', domainId] })
      qc.invalidateQueries({ queryKey: ['history-page', domainId] })
      qc.invalidateQueries({ queryKey: ['domains'] })
      qc.invalidateQueries({ queryKey: ['domains-page'] })
      qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
      showToast({ tone: 'success', text: 'Check triggered.' })
    } catch (err) {
      showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to trigger check.') })
    } finally {
      setChecking(false)
    }
  }

  const isSSLOnly = domain?.check_mode === 'ssl_only'
  const manualSource = isManualSource(domain?.source_type)
  const sourceSummary = formatSourceSummary(domain?.source_type, domain?.source_ref)
  const editingManualSource = editSourceDraft.sourceType === 'manual'
  const editingPreviewName = buildSourceWritePayload(editSourceDraft).name || domain?.name || ''
  const editingSourceSummary = formatSourceSummary(editSourceDraft.sourceType, buildSourceRef(editSourceDraft))
  const activeManualSource = editing ? editingManualSource : manualSource
  const activeSourceType = editing ? editSourceDraft.sourceType : domain?.source_type
  const activeSourceSummary = editing ? editingSourceSummary : sourceSummary
  const showDomainExpirySeries = manualSource && !isSSLOnly
  const editMetadataSplit = splitMetadataBySchema(editMetadata, customFields)
  const visibleDetailMetadata = visibleMetadataSummary(domain?.metadata ?? {}, customFields, 'details')

  // Build chart data from history (reversed, oldest first)
  const chartData = [...historyPreview].reverse().map(c => ({
    time: format(new Date(c.checked_at), 'MM/dd HH:mm'),
    ssl: c.ssl_expiry_days,
    domain: c.registration_check_skipped ? null : c.domain_expiry_days,
  }))
  const historyItems = historyPageData?.items ?? []
  const historyTotalPages = Math.max(1, historyPageData?.total_pages ?? 1)
  const historySafePage = Math.min(historyPage, historyTotalPages)

  if (isLoading) {
    return <DetailSkeleton />
  }

  if (!domain) {
    return (
      <div className="p-6">
        <EmptyState
          icon={Globe}
          title="Domain not found"
          description="The requested inventory record may have been deleted or is no longer visible to your current account."
          action={
            <button className="btn-ghost border border-slate-700" onClick={() => navigate('/domains')}>
              <ArrowLeft size={14} />
              Back to inventory
            </button>
          }
        />
      </div>
    )
  }

  const check = domain.last_check
  const advisoryOnly = Boolean(check?.overall_status === 'ok' && check.status_reasons?.length)
  const operationalReasons = (check?.status_reasons ?? []).filter(reason => reason.severity !== 'advisory')
  const advisoryReasons = (check?.status_reasons ?? []).filter(reason => reason.severity === 'advisory')

  return (
    <div className="p-6 space-y-6">
      <AdHocNotificationModal
        open={notifyOpen}
        domainName={domain.name}
        busy={notifyMutation.isPending}
        results={notificationResults}
        onClose={() => {
          if (notifyMutation.isPending) return
          setNotifyOpen(false)
        }}
        onSubmit={(payload) => notifyMutation.mutate(payload)}
      />

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button className="btn-ghost p-2" onClick={() => navigate('/domains')}>
            <ArrowLeft size={16} />
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-bold text-white">{editing ? editingPreviewName : domain.name}</h1>
              <StatusBadge status={check?.overall_status ?? 'unknown'} title={check?.primary_reason_text} />
              {!activeManualSource && (
                <span className="rounded-full border border-cyan-500/20 bg-cyan-500/10 px-2 py-0.5 text-xs text-cyan-300">
                  {sourceTypeBadge(activeSourceType)}
                </span>
              )}
              {advisoryOnly && (
                <span className="text-xs text-slate-300 bg-slate-500/10 border border-slate-600/40 px-2 py-0.5 rounded-full" title={check?.primary_reason_text}>
                  validation notes
                </span>
              )}
              {domain.tags.map(tag => (
                <span key={tag.toLowerCase()} className="text-xs text-blue-300 bg-blue-500/10 px-2 py-0.5 rounded-full">
                  {tag}
                </span>
              ))}
              {manualSource && domain.check_mode === 'ssl_only' && (
                <span className="text-xs text-violet-400 bg-violet-500/10 px-2 py-0.5 rounded-full">SSL Only</span>
              )}
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-500">
              <span>{sourceTypeLabel(activeSourceType)}</span>
              {activeSourceSummary && <span>| {activeSourceSummary}</span>}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {activeManualSource && (
            <a href={`https://${domain.name}${domain.port && domain.port !== 443 ? `:${domain.port}` : ''}`} target="_blank" rel="noopener noreferrer" className="btn-ghost">
              <ExternalLink size={14} /> Open
            </a>
          )}
          {canEdit && notificationsEnabled && (
            <button
              className="btn-ghost border border-slate-700"
              onClick={() => {
                setNotificationResults([])
                setNotifyOpen(true)
              }}
            >
              <BellRing size={14} />
              Notify
            </button>
          )}
          {canEdit && editing && (
            <>
              <button className="btn-primary" onClick={saveEdit} disabled={updateMutation.isPending}>
                <Check size={14} />
                {updateMutation.isPending ? 'Saving...' : 'Save'}
              </button>
              <button className="btn-ghost border border-slate-700" onClick={() => setEditing(false)} disabled={updateMutation.isPending}>
                <X size={14} />
                Cancel
              </button>
            </>
          )}
          {canEdit && !editing && (
            <button className="btn-ghost border border-slate-700" onClick={startEdit}>
              <Pencil size={13} />
              Edit
            </button>
          )}
          {canEdit && (
            <button className="btn-primary" onClick={handleCheck} disabled={checking}>
              <RefreshCw size={14} className={checking ? 'animate-spin' : ''} />
              {checking ? 'Checking...' : 'Check Now'}
            </button>
          )}
        </div>
      </div>

      {/* Edit extra fields */}
      {editing && (
        <div className="card space-y-4">
          <CollapsiblePanel
            title="Source & identity"
            description="Switch between a manual endpoint, Kubernetes TLS secret, or F5 BIG-IP certificate while keeping one inventory record."
            icon={Globe}
            defaultOpen
            className="border-0 bg-transparent"
            bodyClassName="px-0 pb-0"
          >
            <InventorySourceEditor draft={editSourceDraft} onChange={setEditSourceDraft} />
          </CollapsiblePanel>

          <CollapsiblePanel
            title="Inventory & ownership"
            description="Business context, tags, custom fields, and optional free-form metadata."
            icon={Server}
            defaultOpen={false}
            className="border-0 bg-transparent"
            bodyClassName="px-0 pb-0"
          >
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label className="label">Tags</label>
                <TagEditor tags={editTags} onChange={setEditTags} />
              </div>
              <div>
                <label className="label">Metadata</label>
                <MetadataEditor
                  value={editMetadataSplit.extraMetadata}
                  onChange={extraMetadata => setEditMetadata(mergeSchemaAndExtraMetadata(editMetadataSplit.schemaMetadata, extraMetadata))}
                />
              </div>
            </div>
            <div className="mt-4">
              <CustomFieldInputs fields={customFields} metadata={editMetadata} onChange={setEditMetadata} />
            </div>
          </CollapsiblePanel>

          <CollapsiblePanel
            title="Monitoring policy"
            description={editingManualSource
              ? 'Scheduling, inventory placement, endpoint port, and registration lookup mode.'
              : 'Scheduling, enablement, and folder placement for source-backed certificate tracking.'}
            icon={Clock}
            defaultOpen
            className="border-0 bg-transparent"
            bodyClassName="px-0 pb-0"
          >
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div>
                <label className="label">Check interval (seconds)</label>
                <input className="input" type="number" value={editInterval} onChange={e => setEditInterval(Number(e.target.value))} />
              </div>
              <div>
                <label className="label">Enabled</label>
                <select className="select" value={editEnabled ? '1' : '0'} onChange={e => setEditEnabled(e.target.value === '1')}>
                  <option value="1">Yes</option>
                  <option value="0">No</option>
                </select>
              </div>
              <div>
                <label className="label">Folder</label>
                <select className="select" value={editFolderValue} onChange={e => setEditFolderValue(e.target.value)}>
                  <option value="">No folder</option>
                  {folders.map(folder => (
                    <option key={folder.id} value={folder.id}>{folder.name}</option>
                  ))}
                </select>
              </div>
              {editingManualSource && (
                <>
                  <div>
                    <label className="label">HTTPS Port</label>
                    <input className="input" type="number" min={1} max={65535} value={editPort} onChange={e => setEditPort(Math.max(1, Math.min(65535, Number(e.target.value) || 443)))} />
                  </div>
                  <div>
                    <label className="label">Check Mode</label>
                    <select className="select" value={editCheckMode} onChange={e => setEditCheckMode(e.target.value)}>
                      <option value="full">Full (SSL + Domain Registration)</option>
                      <option value="ssl_only">SSL Only (skip RDAP/WHOIS)</option>
                    </select>
                  </div>
                  <div>
                    <label className="label">DNS Servers</label>
                    <input className="input" value={editDnsServers} onChange={e => setEditDnsServers(e.target.value)} placeholder="10.0.0.1:53, 10.0.0.2:53" />
                  </div>
                </>
              )}
            </div>
            {!editingManualSource && (
              <div className="mt-4 rounded-xl border border-slate-800 bg-slate-900/40 px-4 py-3 text-xs text-slate-400">
                Source-backed inventory items do not use manual endpoint settings such as HTTPS port, per-domain DNS overrides, or RDAP/WHOIS mode.
              </div>
            )}
          </CollapsiblePanel>

          {editingManualSource && (
            <CollapsiblePanel
              title="Trust & certificate overrides"
              description="Optional private trust root used only for this inventory item."
              icon={Shield}
              defaultOpen={false}
              className="border-0 bg-transparent"
              bodyClassName="px-0 pb-0"
            >
              <label className="label">Custom Root CA (PEM)</label>
              <textarea
                className="input h-32 resize-y font-mono text-xs"
                value={editCustomCAPEM}
                onChange={e => setEditCustomCAPEM(e.target.value)}
                placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
              />
            </CollapsiblePanel>
          )}
        </div>
      )}

      {/* Info panels */}
      {operationalReasons.length > 0 && (
        <div className="rounded-2xl border border-amber-500/20 bg-amber-500/5 p-4">
          <div className="flex items-start gap-3">
            <AlertTriangle size={18} className="text-amber-400 mt-0.5" />
            <div className="min-w-0">
              <div className="text-sm font-semibold text-white">Operational alert</div>
              <div className="text-sm text-slate-300 mt-1">{check?.primary_reason_text || 'A non-OK condition is active.'}</div>
              <ul className="mt-3 space-y-2 text-xs text-slate-400">
                {operationalReasons.map(reason => (
                  <li key={`${reason.code}-${reason.summary}`} className="rounded-lg border border-slate-800 bg-slate-900/50 px-3 py-2">
                    <span className="font-medium text-slate-200">[{reason.severity.toUpperCase()}]</span> {reason.detail || reason.summary}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      {advisoryReasons.length > 0 && (
        <div className={`rounded-2xl p-4 ${advisoryOnly ? 'border border-slate-700 bg-slate-900/40' : 'border border-slate-700 bg-slate-900/30'}`}>
          <div className="flex items-start gap-3">
            <AlertTriangle size={18} className="text-slate-400 mt-0.5" />
            <div className="min-w-0">
              <div className="text-sm font-semibold text-white">Validation findings</div>
              <div className="text-sm text-slate-300 mt-1">
                {advisoryOnly ? 'These findings are advisory-only under the current status policy.' : 'Additional validation findings are available for review.'}
              </div>
              <ul className="mt-3 space-y-2 text-xs text-slate-400">
                {advisoryReasons.map(reason => (
                  <li key={`${reason.code}-${reason.summary}`} className="rounded-lg border border-slate-800 bg-slate-900/50 px-3 py-2">
                    <span className="font-medium text-slate-300">[{reason.severity.toUpperCase()}]</span> {reason.detail || reason.summary}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2 xl:grid-cols-4">
        {/* SSL Panel */}
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Shield size={15} className="text-blue-400" /> SSL Certificate
          </h3>
          {check?.ssl_check_error ? (
            <div className="flex items-center gap-2 text-red-400 text-sm">
              <XCircle size={14} /> {check.ssl_check_error}
            </div>
          ) : (
            <>
              <InfoRow label="Subject" value={check?.ssl_subject} />
              <InfoRow label="Issuer" value={check?.ssl_issuer} />
              <InfoRow label="Version" value={check?.ssl_version} />
              <InfoRow
                label="Valid From"
                value={check?.ssl_valid_from ? format(new Date(check.ssl_valid_from), 'yyyy-MM-dd') : undefined}
              />
              <InfoRow
                label="Valid Until"
                value={check?.ssl_valid_until ? format(new Date(check.ssl_valid_until), 'yyyy-MM-dd') : undefined}
              />
              <InfoRow
                label="Expires In"
                value={check?.ssl_expiry_days != null ? `${check.ssl_expiry_days} days` : undefined}
              />
            </>
          )}
        </div>

        {/* SSL Chain Panel */}
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Link2 size={15} className="text-purple-400" /> SSL Chain
          </h3>
          <div className="flex items-center gap-2 mb-3">
            {check?.ssl_chain_valid ? (
              <span className="flex items-center gap-1.5 text-green-400 text-sm">
                <CheckCircle size={14} /> Valid chain ({check.ssl_chain_length} certs)
              </span>
            ) : (
              <span className="flex items-center gap-1.5 text-red-400 text-sm">
                <XCircle size={14} /> Invalid chain
              </span>
            )}
          </div>
          {check?.ssl_chain_error && (
            <p className="text-xs text-yellow-400 mb-2">{check.ssl_chain_error}</p>
          )}
          <InfoRow label="Chain length" value={check?.ssl_chain_length} />
          <InfoRow
            label="Last check"
            value={check?.checked_at ? formatDistanceToNow(new Date(check.checked_at), { addSuffix: true }) : undefined}
          />
          <InfoRow label="Check duration" value={check?.check_duration_ms != null ? `${check.check_duration_ms}ms` : undefined} />
        </div>

        {/* Domain Panel */}
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            {manualSource ? <Globe size={15} className="text-green-400" /> : <Server size={15} className="text-cyan-400" />}
            {manualSource ? 'Domain Registration' : 'Inventory Source'}
            {check?.registration_check_skipped && (
              <span className="text-xs text-violet-400 bg-violet-500/10 px-2 py-0.5 rounded ml-1">skipped</span>
            )}
          </h3>
          {!manualSource ? (
            <div className="space-y-0">
              <InfoRow label="Source type" value={sourceTypeLabel(domain.source_type)} />
              <InfoRow label="Source reference" value={sourceSummary || 'configured'} mono />
              <InfoRow label="Registration lookup" value="Not applicable" />
              <InfoRow label="Reason" value={check?.registration_skip_reason || `source_type=${domain.source_type}`} />
              <InfoRow label="Inventory name" value={domain.name} />
            </div>
          ) : check?.registration_check_skipped ? (
            <div className="space-y-0">
              <InfoRow label="Check mode" value="SSL Only" />
              <InfoRow label="Reason" value={check.registration_skip_reason || 'check_mode=ssl_only'} />
              <InfoRow label="Port" value={domain.port || 443} />
              <InfoRow label="Custom CA" value={domain.custom_ca_pem?.trim() ? 'Configured' : 'Default trust store'} />
              {check.dns_server_used && <InfoRow label="DNS server" value={check.dns_server_used} />}
            </div>
          ) : check?.domain_check_error ? (
            <div className="flex items-center gap-2 text-yellow-400 text-sm">
              <AlertTriangle size={14} /> {check.domain_check_error}
            </div>
          ) : (
            <>
              <InfoRow label="Registrar" value={check?.domain_registrar} />
              <InfoRow label="Status" value={check?.domain_status} />
              <InfoRow label="Source" value={check?.domain_source} />
              <InfoRow label="Port" value={domain.port || 443} />
              <InfoRow label="Custom CA" value={domain.custom_ca_pem?.trim() ? 'Configured' : 'Default trust store'} />
              {check?.dns_server_used && <InfoRow label="DNS server" value={check.dns_server_used} />}
              <InfoRow
                label="Registered"
                value={check?.domain_created_at ? format(new Date(check.domain_created_at), 'yyyy-MM-dd') : undefined}
              />
              <InfoRow
                label="Expires"
                value={check?.domain_expires_at ? format(new Date(check.domain_expires_at), 'yyyy-MM-dd') : undefined}
              />
              <InfoRow
                label="Expires In"
                value={check?.domain_expiry_days != null ? `${check.domain_expiry_days} days` : undefined}
              />
            </>
          )}
        </div>

        {/* Inventory Metadata */}
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Server size={15} className="text-cyan-400" /> Inventory Metadata
          </h3>
          <InfoRow label="Tags" value={domain.tags.join(', ')} />
          {visibleDetailMetadata.map(item => (
            <InfoRow key={item.key} label={item.label} value={item.value} />
          ))}
          {Object.keys(domain.metadata ?? {}).length === 0 ? (
            <div className="text-sm text-gray-500">No metadata configured.</div>
          ) : (
            Object.entries(domain.metadata)
              .filter(([key]) => !visibleDetailMetadata.some(item => item.key === key))
              .sort(([a], [b]) => a.localeCompare(b))
              .map(([key, value]) => (
                <InfoRow key={key} label={key} value={value} mono />
              ))
          )}
        </div>
      </div>

      {/* Chart */}
      {chartData.length > 1 && (
        <div className="card">
          <h3 className="font-semibold text-white mb-4 flex items-center gap-2">
            <Clock size={15} className="text-gray-400" /> Expiry History
          </h3>
          <Suspense fallback={<Skeleton className="h-56 w-full" />}>
            <ExpiryHistoryChart data={chartData} showDomain={showDomainExpirySeries} />
          </Suspense>
        </div>
      )}

      {!editing && Object.keys(domain.metadata ?? {}).length > 0 && (
        <div className="rounded-xl border border-gray-800 bg-gray-900/40 p-4 text-xs text-gray-400">
          Search text: <span className="font-mono text-gray-300">{metadataSearchText(domain.metadata)}</span>
        </div>
      )}

      {/* HTTP and advanced security checks */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Server size={15} className="text-blue-400" /> HTTP/HTTPS
          </h3>
          {!manualSource ? (
            <p className="text-sm text-gray-500">Source-backed inventory items skip live HTTP probing and read certificate metadata directly from the configured source.</p>
          ) : check ? (
            <>
              <InfoRow label="HTTP status" value={check.http_status_code || undefined} />
              <InfoRow label="Response time" value={check.http_response_time_ms ? `${check.http_response_time_ms}ms` : undefined} />
              <InfoRow label="Redirect HTTP -> HTTPS" value={check.http_redirects_https ? 'Yes' : 'No'} />
              <InfoRow label="HSTS enabled" value={check.http_hsts_enabled ? 'Yes' : 'No'} />
              <InfoRow label="HSTS max-age" value={check.http_hsts_max_age || undefined} />
              <InfoRow label="Final URL" value={check.http_final_url || undefined} />
              <InfoRow label="HTTP error" value={check.http_error || undefined} />
            </>
          ) : (
            <p className="text-sm text-gray-500">No checks yet.</p>
          )}
        </div>

        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Shield size={15} className="text-emerald-400" /> Advanced Security
          </h3>
          {!manualSource ? (
            <p className="text-sm text-gray-500">Revocation, CAA, cipher, and HTTP validation are not run for source-backed inventory items in v1.4.0.</p>
          ) : check ? (
            <>
              <InfoRow label="Cipher grade" value={check.cipher_grade || undefined} />
              <InfoRow label="Cipher weak" value={check.cipher_weak ? 'Yes' : 'No'} />
              <InfoRow label="Cipher details" value={check.cipher_details || check.cipher_weak_reason || undefined} />
              <InfoRow label="OCSP status" value={check.ocsp_status || undefined} />
              <InfoRow label="OCSP error" value={check.ocsp_error || undefined} />
              <InfoRow label="CRL status" value={check.crl_status || undefined} />
              <InfoRow label="CRL error" value={check.crl_error || undefined} />
              <InfoRow label="CAA present" value={check.caa_present ? 'Yes' : (check.caa || check.caa_error ? 'No' : undefined)} />
              <InfoRow label="CAA records" value={check.caa || undefined} />
              <InfoRow label="CAA lookup domain" value={check.caa_query_domain || undefined} />
              <InfoRow label="CAA error" value={check.caa_error || undefined} />
            </>
          ) : (
            <p className="text-sm text-gray-500">No checks yet.</p>
          )}
        </div>
      </div>

      {/* SSL Chain details */}
      {check?.ssl_chain_details && check.ssl_chain_details.length > 0 && (
        <div className="card">
          <h3 className="font-semibold text-white mb-4 flex items-center gap-2">
            <Link2 size={15} className="text-purple-400" /> Certificate Chain Details
          </h3>
          <div className="space-y-2">
            {check.ssl_chain_details.map((cert, i) => (
              <ChainCard key={i} cert={cert} index={i} />
            ))}
          </div>
        </div>
      )}

      {/* Check history table */}
      {historyItems.length > 0 && (
        <div className="card">
          <h3 className="font-semibold text-white mb-4 flex items-center gap-2">
            <Clock size={15} className="text-gray-400" /> Check History
          </h3>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-800">
                  <th className="pb-2 font-medium">Time</th>
                  <th className="pb-2 font-medium">Status</th>
                  <th className="pb-2 font-medium">SSL Expiry</th>
                  <th className="pb-2 font-medium">Domain Expiry</th>
                  <th className="pb-2 font-medium">Chain</th>
                  <th className="pb-2 font-medium">Duration</th>
                  <th className="pb-2 font-medium">Reason</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {historyItems.map(c => (
                  <tr key={c.id} className="hover:bg-gray-800/30">
                    <td className="py-2 text-gray-400">
                      {format(new Date(c.checked_at), 'yyyy-MM-dd HH:mm')}
                    </td>
                    <td className="py-2">
                      <StatusBadge status={c.overall_status} title={c.primary_reason_text} />
                    </td>
                    <td className="py-2 text-gray-300">
                      {c.ssl_expiry_days != null ? `${c.ssl_expiry_days}d` : '-'}
                    </td>
                    <td className="py-2 text-gray-300">
                      {c.registration_check_skipped ? <span className="text-gray-600">N/A</span> : c.domain_expiry_days != null ? `${c.domain_expiry_days}d` : '-'}
                    </td>
                    <td className="py-2">
                      {c.ssl_check_error ? (
                        <XCircle size={12} className="text-red-400" />
                      ) : c.ssl_chain_valid ? (
                        <CheckCircle size={12} className="text-green-400" />
                      ) : (
                        <AlertTriangle size={12} className="text-yellow-400" />
                      )}
                    </td>
                    <td className="py-2 text-gray-500">{c.check_duration_ms}ms</td>
                    <td className="py-2 text-gray-400 max-w-[22rem]">{c.primary_reason_text || '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination
            page={historySafePage}
            totalPages={historyTotalPages}
            onPageChange={setHistoryPage}
            summary={`Showing ${(historySafePage - 1) * (historyPageData?.page_size ?? 20) + 1}-${Math.min(historySafePage * (historyPageData?.page_size ?? 20), historyPageData?.total ?? historyItems.length)} of ${historyPageData?.total ?? historyItems.length} checks`}
          />
        </div>
      )}
    </div>
  )
}
