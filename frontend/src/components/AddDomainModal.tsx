import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import { Clock3, FileJson, FolderPlus, ListChecks, Plus, Server, Settings2, Shield, Upload } from 'lucide-react'
import { createDomain, fetchFolders, createFolder, fetchBootstrap, fetchCustomFields, importDomains } from '../api/client'
import type { DomainImportRequest, DomainImportResponse, DomainWritePayload } from '../types'
import TagEditor from './TagEditor'
import MetadataEditor from './MetadataEditor'
import { normalizeMetadata } from '../lib/domainFields'
import { mergeSchemaAndExtraMetadata, splitMetadataBySchema } from '../lib/customFields'
import CustomFieldInputs from './CustomFieldInputs'
import CollapsiblePanel from './CollapsiblePanel'
import ModalShell from './ModalShell'
import InventorySourceEditor from './InventorySourceEditor'
import { buildSourceWritePayload, createInventorySourceDraft } from '../lib/domainSources'

interface Props {
  onClose: () => void
  initialDraft?: Partial<DomainWritePayload> | null
}

type Mode = 'single' | 'lines' | 'json'

const INTERVALS = [
  { label: '1 hour', value: 3600 },
  { label: '3 hours', value: 10800 },
  { label: '6 hours', value: 21600 },
  { label: '12 hours', value: 43200 },
  { label: '24 hours', value: 86400 },
]

export default function AddDomainModal({ onClose, initialDraft }: Props) {
  const [mode, setMode] = useState<Mode>('single')
  const [sourceDraft, setSourceDraft] = useState(() => createInventorySourceDraft(initialDraft))
  const [tags, setTags] = useState<string[]>([])
  const [metadata, setMetadata] = useState<Record<string, string>>({})
  const [enabled, setEnabled] = useState(true)
  const [interval, setInterval] = useState(21600)
  const [port, setPort] = useState(443)
  const [folderValue, setFolderValue] = useState<string>('')
  const [bulkText, setBulkText] = useState('')
  const [jsonText, setJsonText] = useState('')
  const [customCAPEM, setCustomCAPEM] = useState('')
  const [checkMode, setCheckMode] = useState('')
  const [dnsServers, setDnsServers] = useState('')
  const [submitError, setSubmitError] = useState('')
  const [newFolderName, setNewFolderName] = useState('')
  const [importMode, setImportMode] = useState<'create_only' | 'upsert'>('create_only')
  const [dryRun, setDryRun] = useState(false)
  const [triggerChecks, setTriggerChecks] = useState(false)
  const [lastImport, setLastImport] = useState<DomainImportResponse | null>(null)

  const qc = useQueryClient()
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })
  const { data: cfg } = useQuery({ queryKey: ['bootstrap'], queryFn: fetchBootstrap })
  const { data: customFields = [] } = useQuery({ queryKey: ['custom-fields'], queryFn: () => fetchCustomFields(false) })

  const mutation = useMutation({
    mutationFn: createDomain,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domains'] })
      qc.invalidateQueries({ queryKey: ['domains-page'] })
      qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
      qc.invalidateQueries({ queryKey: ['tags'] })
    },
  })

  const importMutation = useMutation({
    mutationFn: importDomains,
    onSuccess: (result) => {
      setLastImport(result)
      if (!result.dry_run) {
        qc.invalidateQueries({ queryKey: ['domains'] })
        qc.invalidateQueries({ queryKey: ['domains-page'] })
        qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
        qc.invalidateQueries({ queryKey: ['summary'] })
        qc.invalidateQueries({ queryKey: ['tags'] })
      }
    },
  })

  const folderMutation = useMutation({
    mutationFn: createFolder,
    onSuccess: (folder) => {
      qc.invalidateQueries({ queryKey: ['folders'] })
      setFolderValue(String(folder.id))
      setNewFolderName('')
    },
  })

  const selectedFolderID = folderValue ? Number(folderValue) : undefined
  const effectiveMode = checkMode || cfg?.domains.default_check_mode || 'full'
  const metadataSplit = useMemo(() => splitMetadataBySchema(metadata, customFields), [customFields, metadata])
  const importDefaults = useMemo(() => buildImportDefaults({
    tags,
    metadata,
    enabled,
    interval,
    port,
    folderID: selectedFolderID,
    customCAPEM,
    checkMode: effectiveMode,
    dnsServers,
  }), [tags, metadata, enabled, interval, port, selectedFolderID, customCAPEM, effectiveMode, dnsServers])

  const handleSingle = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitError('')
    setLastImport(null)

    try {
      const sourcePayload = buildSourceWritePayload(sourceDraft)
      if (sourcePayload.source_type === 'manual' && !sourcePayload.name?.trim()) {
        setSubmitError('Domain / host is required for manual endpoints.')
        return
      }

      await mutation.mutateAsync({
        name: sourcePayload.name,
        source_type: sourcePayload.source_type,
        source_ref: sourcePayload.source_ref,
        tags,
        metadata: normalizeMetadata(metadata),
        enabled,
        check_interval: interval,
        folder_id: selectedFolderID,
        ...(sourcePayload.source_type === 'manual'
          ? {
              port,
              custom_ca_pem: customCAPEM.trim() || undefined,
              check_mode: effectiveMode,
              dns_servers: dnsServers.trim() || undefined,
            }
          : {}),
      })
      onClose()
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const handleLinesImport = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitError('')
    setLastImport(null)

    const domains = bulkText
      .split('\n')
      .map(line => line.trim())
      .filter(line => line.length > 0 && !line.startsWith('#'))
      .map(line => ({ name: line }))

    if (domains.length === 0) {
      setSubmitError('Add at least one domain to import.')
      return
    }

    try {
      const result = await importMutation.mutateAsync({
        mode: importMode,
        dry_run: dryRun,
        trigger_checks: triggerChecks,
        defaults: importDefaults,
        domains,
      })
      if (!result.dry_run && result.summary.failed === 0 && result.summary.skipped === 0) {
        onClose()
      }
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const handleJSONImport = async (e: React.FormEvent) => {
    e.preventDefault()
    setSubmitError('')
    setLastImport(null)

    try {
      const payload = parseJSONImportText(jsonText)
      const finalPayload: DomainImportRequest = {
        mode: payload.mode ?? importMode,
        dry_run: payload.dry_run ?? dryRun,
        trigger_checks: payload.trigger_checks ?? triggerChecks,
        defaults: {
          ...importDefaults,
          ...(payload.defaults ?? {}),
        },
        domains: payload.domains,
      }

      const result = await importMutation.mutateAsync(finalPayload)
      if (!result.dry_run && result.summary.failed === 0 && result.summary.skipped === 0) {
        onClose()
      }
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const loadCertFile = async (file: File | null) => {
    if (!file) return
    try {
      const content = await file.text()
      setCustomCAPEM(content)
    } catch {
      // ignore read errors for now
    }
  }

  const handleCreateFolder = async () => {
    const trimmed = newFolderName.trim()
    if (!trimmed) return
    setSubmitError('')
    try {
      await folderMutation.mutateAsync(trimmed)
    } catch (err) {
      setSubmitError(extractErrorMessage(err))
    }
  }

  const isImportMode = mode !== 'single'
  const isBusy = mutation.isPending || importMutation.isPending
  const singleModeUsesManualEndpoint = sourceDraft.sourceType === 'manual'

  return (
    <ModalShell
      onClose={onClose}
      panelClassName="my-4 flex max-h-[calc(100vh-2rem)] max-w-5xl flex-col overflow-hidden"
      title={<h2 id="add-domains-title">Add Domains</h2>}
      bodyClassName="overflow-y-auto"
    >
          <div className="mb-5 grid grid-cols-1 gap-2 md:grid-cols-3">
            <button
              className={`btn text-xs ${mode === 'single' ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
              onClick={() => setMode('single')}
            >
              Single domain
            </button>
            <button
              className={`btn text-xs ${mode === 'lines' ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
              onClick={() => setMode('lines')}
            >
              <ListChecks size={14} />
              Line import
            </button>
            <button
              className={`btn text-xs ${mode === 'json' ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
              onClick={() => setMode('json')}
            >
              <FileJson size={14} />
              JSON import
            </button>
          </div>

          <form onSubmit={mode === 'single' ? handleSingle : mode === 'lines' ? handleLinesImport : handleJSONImport} className="space-y-5">
            <CollapsiblePanel
              title={mode === 'single' ? 'Single domain source' : mode === 'lines' ? 'Line import source' : 'JSON import source'}
              description={mode === 'single'
                ? 'Create one monitored inventory item from a manual endpoint, Kubernetes TLS secret, or F5 certificate.'
                : mode === 'lines'
                  ? 'Import a plain list of domains while applying the defaults below.'
                  : 'Import structured records with metadata, tags, and custom inventory fields.'}
              icon={mode === 'single' ? Server : mode === 'lines' ? ListChecks : FileJson}
              defaultOpen
            >
              {mode === 'single' && (
                <InventorySourceEditor draft={sourceDraft} onChange={setSourceDraft} />
              )}

              {mode === 'lines' && (
                <div>
                  <label className="label" htmlFor="bulk-domain-lines">Domains (one per line)</label>
                  <textarea
                    id="bulk-domain-lines"
                    className="input h-36 resize-y font-mono"
                    placeholder="example.com&#10;another.org&#10;# comments are ignored"
                    value={bulkText}
                    onChange={e => setBulkText(e.target.value)}
                    required
                  />
                  <p className="mt-1 text-xs text-gray-500">Shared defaults below apply to every imported domain.</p>
                </div>
              )}

              {mode === 'json' && (
                <div>
                  <label className="label" htmlFor="json-domain-payload">JSON payload</label>
                  <textarea
                    id="json-domain-payload"
                    className="input h-64 resize-y font-mono text-xs"
                    placeholder={JSON_PLACEHOLDER}
                    value={jsonText}
                    onChange={e => setJsonText(e.target.value)}
                    required
                  />
                  <p className="mt-1 text-xs text-gray-500">Supports top-level objects with a `domains` array or a plain array. Unknown item fields are stored in metadata automatically, and source-backed imports can pass `source_type` plus `source_ref`.</p>
                </div>
              )}
            </CollapsiblePanel>

            <CollapsiblePanel
              title="Inventory & ownership"
              description="Tags, schema-driven custom fields, and extra metadata for business context."
              icon={Server}
              defaultOpen
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div>
                  <label className="label">Tags</label>
                  <TagEditor tags={tags} onChange={setTags} placeholder="production api owner-team" />
                </div>

                <div>
                  <label className="label">Metadata</label>
                  <MetadataEditor
                    value={metadataSplit.extraMetadata}
                    onChange={extraMetadata => setMetadata(mergeSchemaAndExtraMetadata(metadataSplit.schemaMetadata, extraMetadata))}
                  />
                </div>
              </div>

              <div className="mt-4">
                <CustomFieldInputs fields={customFields} metadata={metadata} onChange={setMetadata} />
              </div>
            </CollapsiblePanel>

            <CollapsiblePanel
              title="Monitoring defaults"
              description={mode === 'single' && !singleModeUsesManualEndpoint
                ? 'Scheduling, inventory placement, and enablement for source-backed certificate tracking.'
                : 'Scheduling, endpoint port, folder placement, registration lookup policy, and DNS overrides.'}
              icon={Clock3}
              defaultOpen
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                <div>
                  <label className="label">Check interval</label>
                  <select className="select" value={interval} onChange={e => setInterval(Number(e.target.value))}>
                    {INTERVALS.map(item => (
                      <option key={item.value} value={item.value}>{item.label}</option>
                    ))}
                  </select>
                </div>

                <div>
                  <label className="label">Enabled</label>
                  <select className="select" value={enabled ? 'true' : 'false'} onChange={e => setEnabled(e.target.value === 'true')}>
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </select>
                </div>

                <div>
                  <label className="label">Folder</label>
                  <select className="select" value={folderValue} onChange={e => setFolderValue(e.target.value)}>
                    <option value="">No folder</option>
                    {folders.map(folder => (
                      <option key={folder.id} value={folder.id}>{folder.name}</option>
                    ))}
                  </select>
                </div>

                {(mode !== 'single' || singleModeUsesManualEndpoint) && (
                  <>
                    <div>
                      <label className="label">HTTPS Port</label>
                      <input
                        className="input"
                        type="number"
                        min={1}
                        max={65535}
                        value={port}
                        onChange={e => setPort(Math.max(1, Math.min(65535, Number(e.target.value) || 443)))}
                      />
                    </div>

                    <div>
                      <label className="label">Check mode</label>
                      <select className="select" value={checkMode} onChange={e => setCheckMode(e.target.value)}>
                        <option value="">Default ({cfg?.domains.default_check_mode || 'full'})</option>
                        <option value="full">Full (SSL + Domain Registration)</option>
                        <option value="ssl_only">SSL Only (skip RDAP/WHOIS)</option>
                      </select>
                      <p className="mt-1 text-xs text-gray-500">SSL Only skips domain registration lookup for internal names such as `.local` or `.internal`.</p>
                    </div>

                    <div>
                      <label className="label">DNS Servers</label>
                      <input
                        className="input"
                        placeholder="10.0.0.1:53, 10.0.0.2:53"
                        value={dnsServers}
                        onChange={e => setDnsServers(e.target.value)}
                      />
                      <p className="mt-1 text-xs text-gray-500">Per-domain DNS servers. Overrides global DNS config.</p>
                    </div>
                  </>
                )}
              </div>

              {mode === 'single' && !singleModeUsesManualEndpoint && (
                <div className="mt-4 rounded-xl border border-slate-800 bg-slate-900/40 px-4 py-3 text-xs text-slate-400">
                  Source-backed inventory items do not use manual endpoint settings such as port, DNS override, custom CA, or RDAP/WHOIS check mode.
                </div>
              )}
            </CollapsiblePanel>

            {isImportMode && (
              <CollapsiblePanel
                title="Import execution"
                description="Control upsert behavior, validation-only runs, and post-import checks."
                icon={Settings2}
                defaultOpen
              >
                <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
                  <div>
                    <label className="label">Import mode</label>
                    <select className="select" value={importMode} onChange={e => setImportMode(e.target.value as 'create_only' | 'upsert')}>
                      <option value="create_only">Create only</option>
                      <option value="upsert">Upsert existing domains</option>
                    </select>
                  </div>
                  <label className="flex items-center gap-2 rounded-lg border border-gray-800 px-3 py-2 text-sm text-gray-300">
                    <input className="checkbox" type="checkbox" checked={dryRun} onChange={e => setDryRun(e.target.checked)} />
                    Dry run only
                  </label>
                  <label className="flex items-center gap-2 rounded-lg border border-gray-800 px-3 py-2 text-sm text-gray-300">
                    <input className="checkbox" type="checkbox" checked={triggerChecks} onChange={e => setTriggerChecks(e.target.checked)} />
                    Trigger checks after import
                  </label>
                </div>
                <div className="text-xs text-gray-500">
                  Shared defaults in this form apply to every imported record. In `upsert` mode, provided defaults can overwrite existing domain settings.
                </div>
              </CollapsiblePanel>
            )}

            <CollapsiblePanel
              title="Trust & inventory helpers"
              description="Folder bootstrap helpers and private trust material for internal PKI environments."
              icon={Shield}
              defaultOpen={false}
            >
              <div className="grid grid-cols-1 items-end gap-2 md:grid-cols-[1fr_auto]">
                <div>
                  <label className="label">Create folder</label>
                  <input
                    className="input"
                    placeholder="backend, public-sites, staging"
                    value={newFolderName}
                    onChange={e => setNewFolderName(e.target.value)}
                  />
                </div>
                <button type="button" className="btn-ghost border border-gray-700" onClick={handleCreateFolder} disabled={folderMutation.isPending}>
                  <FolderPlus size={14} />
                  {folderMutation.isPending ? 'Creating...' : 'Create'}
                </button>
              </div>

              <div className="mt-4">
                {(mode !== 'single' || singleModeUsesManualEndpoint) ? (
                  <>
                    <div className="mb-1.5 flex items-center justify-between">
                      <label className="label mb-0">Custom Root CA (optional)</label>
                      <label className="inline-flex cursor-pointer items-center gap-1.5 border border-gray-700 px-2 py-1 text-xs btn-ghost">
                        <Upload size={12} />
                        Load .crt/.pem
                        <input
                          type="file"
                          accept=".crt,.pem,.cer,text/plain"
                          className="hidden"
                          onChange={e => loadCertFile(e.target.files?.[0] ?? null)}
                        />
                      </label>
                    </div>
                    <textarea
                      className="input h-36 resize-y font-mono text-xs"
                      placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
                      value={customCAPEM}
                      onChange={e => setCustomCAPEM(e.target.value)}
                    />
                    <p className="mt-1 text-xs text-gray-500">Used as an additional trust root for SSL and HTTPS checks for this domain.</p>
                  </>
                ) : (
                  <div className="rounded-xl border border-slate-800 bg-slate-900/40 px-4 py-3 text-xs text-slate-400">
                    Source-backed inventory items read certificate metadata directly from Kubernetes or F5 and do not consume a per-item custom CA bundle.
                  </div>
                )}
              </div>
            </CollapsiblePanel>

            {submitError && (
              <div className="rounded-lg border border-amber-600/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-300">
                {submitError}
              </div>
            )}

            {lastImport && (
              <div className="rounded-xl border border-gray-800 bg-gray-950/40 p-4 text-sm">
                <div className="mb-3 flex flex-wrap items-center gap-2">
                  <span className="rounded-full bg-blue-500/10 px-2 py-1 text-xs text-blue-300">total {lastImport.summary.total}</span>
                  <span className="rounded-full bg-green-500/10 px-2 py-1 text-xs text-green-300">created {lastImport.summary.created}</span>
                  <span className="rounded-full bg-cyan-500/10 px-2 py-1 text-xs text-cyan-300">updated {lastImport.summary.updated}</span>
                  <span className="rounded-full bg-amber-500/10 px-2 py-1 text-xs text-amber-300">skipped {lastImport.summary.skipped}</span>
                  <span className="rounded-full bg-red-500/10 px-2 py-1 text-xs text-red-300">failed {lastImport.summary.failed}</span>
                  {lastImport.dry_run && (
                    <span className="rounded-full bg-violet-500/10 px-2 py-1 text-xs text-violet-300">dry run</span>
                  )}
                </div>
                <div className="max-h-48 space-y-2 overflow-y-auto">
                  {lastImport.results.map(result => (
                    <div key={`${result.index}-${result.name ?? 'unknown'}`} className="rounded-lg border border-gray-800 px-3 py-2 text-xs">
                      <div className="flex items-center justify-between gap-3">
                        <span className="font-medium text-gray-200">{result.name || `Item ${result.index + 1}`}</span>
                        <span className="text-gray-500">{result.action}</span>
                      </div>
                      {result.error && (
                        <div className="mt-1 text-red-400">{result.error}</div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            <div className="flex gap-3 pt-2">
              <button type="button" onClick={onClose} className="btn-ghost flex-1 border border-gray-700">
                Cancel
              </button>
              <button type="submit" className="btn-primary flex-1" disabled={isBusy}>
                <Plus size={15} />
                {isBusy ? 'Working...' : mode === 'single' ? 'Add Domain' : dryRun ? 'Validate Import' : 'Run Import'}
              </button>
            </div>
          </form>
    </ModalShell>
  )
}

function buildImportDefaults(args: {
  tags: string[]
  metadata: Record<string, string>
  enabled: boolean
  interval: number
  port: number
  folderID?: number
  customCAPEM: string
  checkMode: string
  dnsServers: string
}): Record<string, unknown> {
  const out: Record<string, unknown> = {
    enabled: args.enabled,
    check_interval: args.interval,
    port: args.port,
    check_mode: args.checkMode,
  }

  if (args.tags.length > 0) out.tags = args.tags
  const metadata = normalizeMetadata(args.metadata)
  if (Object.keys(metadata).length > 0) out.metadata = metadata
  if (args.folderID) out.folder_id = args.folderID
  if (args.customCAPEM.trim()) out.custom_ca_pem = args.customCAPEM.trim()
  if (args.dnsServers.trim()) out.dns_servers = args.dnsServers.trim()

  return out
}

function parseJSONImportText(text: string): DomainImportRequest {
  const parsed = JSON.parse(text) as unknown
  if (Array.isArray(parsed)) {
    return { domains: normalizeImportDomains(parsed) }
  }
  if (!parsed || typeof parsed !== 'object') {
    throw new Error('JSON import must be an array or an object with a domains field.')
  }

  const payload = parsed as Record<string, unknown>
  const domains = payload.domains
  if (!Array.isArray(domains)) {
    throw new Error('JSON import object must contain a domains array.')
  }

  return {
    mode: payload.mode as 'create_only' | 'upsert' | undefined,
    dry_run: typeof payload.dry_run === 'boolean' ? payload.dry_run : undefined,
    trigger_checks: typeof payload.trigger_checks === 'boolean' ? payload.trigger_checks : undefined,
    defaults: (payload.defaults as Record<string, unknown> | undefined) ?? undefined,
    domains: normalizeImportDomains(domains),
  }
}

function normalizeImportDomains(items: unknown[]): Array<Record<string, unknown>> {
  return items.map((item) => {
    if (typeof item === 'string') {
      return { name: item }
    }
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      throw new Error('Each imported domain entry must be a string or object.')
    }
    return item as Record<string, unknown>
  })
}

function extractErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status
    const backendMessage = (err.response?.data as { error?: string } | undefined)?.error

    if (status === 409) {
      return backendMessage || 'Domain already exists.'
    }
    if (backendMessage) {
      return backendMessage
    }
  }

  if (err instanceof Error) {
    return err.message
  }

  return 'Request failed. Please try again.'
}

const JSON_PLACEHOLDER = `{
  "mode": "upsert",
  "dry_run": true,
  "trigger_checks": false,
  "defaults": {
    "tags": ["enterprise", "prod"],
    "metadata": {
      "owner": "Platform Team",
      "zone": "corp"
    },
    "check_mode": "ssl_only"
  },
  "domains": [
    {
      "domain": "example.com",
      "type": "public",
      "owner": "Web Team"
    },
    {
      "domain": "vcenter.local",
      "metadata": {
        "owner": "Virtualization Team",
        "change_id": "CHG-001"
      }
    }
  ]
}`
