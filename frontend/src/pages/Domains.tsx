import { useEffect, useMemo, useRef, useState } from 'react'
import { QueryClient, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  Plus,
  Trash2,
  RefreshCw,
  ExternalLink,
  Tag,
  ChevronDown,
  ChevronRight,
  Download,
  FolderPlus,
  ArrowUp,
  ArrowDown,
  Save,
  SlidersHorizontal,
  FolderX,
  Settings2,
} from 'lucide-react'
import {
  createFolder,
  deleteDomain,
  deleteFolder,
  exportDomainsCsvUrl,
  fetchCustomFields,
  fetchDomains,
  fetchDomainsPage,
  fetchFolders,
  reorderDomains as saveDomainOrder,
  fetchSummary,
  triggerCheck,
  updateFolder,
} from '../api/client'
import StatusBadge from '../components/StatusBadge'
import ExpiryBar from '../components/ExpiryBar'
import AddDomainModal from '../components/AddDomainModal'
import ConfirmDialog from '../components/ConfirmDialog'
import EmptyState from '../components/EmptyState'
import Pagination from '../components/Pagination'
import { ListCardSkeleton, PageHeadingSkeleton } from '../components/Skeleton'
import { useToast } from '../components/ToastProvider'
import VirtualizedCardList from '../components/VirtualizedCardList'
import { formatDistanceToNow } from 'date-fns'
import type { AuthMe, BootstrapConfig, Domain, Folder } from '../types'
import { metadataSearchText, tagsToText } from '../lib/domainFields'
import { filterableCustomFields, visibleMetadataSummary } from '../lib/customFields'
import { useDebouncedValue } from '../lib/useDebouncedValue'
import { activateCardOnKey, getErrorMessage, isTypingTarget, parseOptionalInt, updateFilter } from '../lib/utils'

type SortMode = 'custom' | 'name' | 'status' | 'created_at' | 'domain_expiry' | 'ssl_expiry' | 'last_check'

type DomainsProps = {
  me?: AuthMe
  bootstrap?: BootstrapConfig
}

export default function Domains({ me, bootstrap }: DomainsProps) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { showToast } = useToast()
  const searchRef = useRef<HTMLInputElement | null>(null)
  const canEdit = me?.can_edit ?? false

  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [selectedFolder, setSelectedFolder] = useState<string>('all')
  const [sortMode, setSortMode] = useState<SortMode>('custom')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [sslExpiryWithin, setSSLExpiryWithin] = useState('')
  const [domainExpiryWithin, setDomainExpiryWithin] = useState('')
  const [metadataFilters, setMetadataFilters] = useState<Record<string, string>>({})
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [foldersOpen, setFoldersOpen] = useState(false)
  const [checkingId, setCheckingId] = useState<number | null>(null)
  const [folderName, setFolderName] = useState('')
  const [folderDrafts, setFolderDrafts] = useState<Record<number, string>>({})
  const [confirmState, setConfirmState] = useState<
    | { kind: 'domain'; id: number; name: string }
    | { kind: 'folder'; folder: Folder }
    | null
  >(null)
  const debouncedSearch = useDebouncedValue(search, 250)

  const filters = useMemo(() => ({
    search: debouncedSearch.trim() || undefined,
    status: statusFilter !== 'all' ? statusFilter : undefined,
    folder_id: selectedFolder !== 'all' ? Number(selectedFolder) : undefined,
    metadata_filters: Object.keys(metadataFilters).length > 0 ? metadataFilters : undefined,
    ssl_expiry_lte: parseOptionalInt(sslExpiryWithin),
    domain_expiry_lte: parseOptionalInt(domainExpiryWithin),
    sort_by: sortMode,
    sort_dir: sortDir,
    page,
    page_size: pageSize,
  }), [debouncedSearch, domainExpiryWithin, metadataFilters, page, pageSize, selectedFolder, sortDir, sortMode, sslExpiryWithin, statusFilter])

  const { data: pageData, isLoading } = useQuery({
    queryKey: ['domains-page', filters],
    queryFn: () => fetchDomainsPage(filters),
    placeholderData: previous => previous,
  })
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })
  const { data: summary } = useQuery({ queryKey: ['summary'], queryFn: fetchSummary })
  const { data: customFields = [] } = useQuery({ queryKey: ['custom-fields'], queryFn: () => fetchCustomFields(false) })
  const { data: reorderSourceDomains = [] } = useQuery({
    queryKey: ['domains-reorder', canEdit, sortMode],
    queryFn: fetchDomains,
    enabled: canEdit && sortMode === 'custom',
  })

  const domains = pageData?.items ?? []
  const total = pageData?.total ?? 0
  const totalPages = Math.max(1, pageData?.total_pages ?? 1)
  const safePage = Math.min(page, totalPages)
  const filterableFields = filterableCustomFields(customFields)
  const visibleTableFields = customFields.filter(field => field.enabled && field.visible_in_table)

  const sslWarning = bootstrap?.alerts.ssl_expiry_warning_days ?? 14
  const sslCritical = bootstrap?.alerts.ssl_expiry_critical_days ?? 3
  const domainWarning = bootstrap?.alerts.domain_expiry_warning_days ?? 30
  const domainCritical = bootstrap?.alerts.domain_expiry_critical_days ?? 7

  const deleteMutation = useMutation({
    mutationFn: deleteDomain,
    onSuccess: () => {
      invalidateDomainQueries(qc)
      showToast({ tone: 'success', text: 'Domain deleted.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to delete domain.') }),
  })

  const folderCreateMutation = useMutation({
    mutationFn: createFolder,
    onSuccess: (folder) => {
      setFolderName('')
      setSelectedFolder(String(folder.id))
      qc.invalidateQueries({ queryKey: ['folders'] })
      showToast({ tone: 'success', text: `Folder "${folder.name}" created.` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to create folder.') }),
  })

  const folderUpdateMutation = useMutation({
    mutationFn: ({ id, name }: { id: number; name: string }) => updateFolder(id, name),
    onSuccess: (folder) => {
      setFolderDrafts((drafts) => ({ ...drafts, [folder.id]: folder.name }))
      qc.invalidateQueries({ queryKey: ['folders'] })
      showToast({ tone: 'success', text: `Folder "${folder.name}" updated.` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to rename folder.') }),
  })

  const folderDeleteMutation = useMutation({
    mutationFn: deleteFolder,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['folders'] })
      qc.invalidateQueries({ queryKey: ['domains-page'] })
      setSelectedFolder('all')
      showToast({ tone: 'success', text: 'Folder deleted. Domains were moved to "No folder".' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to delete folder.') }),
  })

  useEffect(() => {
    setPage(1)
  }, [search, statusFilter, selectedFolder, sortMode, sortDir, pageSize, sslExpiryWithin, domainExpiryWithin, metadataFilters])

  useEffect(() => {
    setFolderDrafts((current) => {
      const next: Record<number, string> = {}
      folders.forEach((folder) => {
        next[folder.id] = current[folder.id] ?? folder.name
      })
      return next
    })
  }, [folders])

  useEffect(() => {
    if (!pageData) return

    const prefetch = (targetPage: number) => {
      if (targetPage < 1 || targetPage > totalPages || targetPage === safePage) return
      const nextFilters = { ...filters, page: targetPage }
      qc.prefetchQuery({
        queryKey: ['domains-page', nextFilters],
        queryFn: () => fetchDomainsPage(nextFilters),
      })
    }

    prefetch(safePage + 1)
    prefetch(safePage - 1)
  }, [filters, pageData, qc, safePage, totalPages])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k' && !isTypingTarget(event.target)) {
        event.preventDefault()
        searchRef.current?.focus()
        searchRef.current?.select()
        return
      }
      if (event.key === 'Escape') {
        setFiltersOpen(false)
        setFoldersOpen(false)
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const handleCheck = async (e: React.MouseEvent, id: number) => {
    e.stopPropagation()
    setCheckingId(id)
    try {
      await triggerCheck(id)
      invalidateDomainQueries(qc)
      showToast({ tone: 'success', text: 'Check triggered.' })
    } catch (err) {
      showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to trigger check.') })
    } finally {
      setCheckingId(null)
    }
  }

  const handleDelete = (e: React.MouseEvent, id: number, name: string) => {
    e.stopPropagation()
    setConfirmState({ kind: 'domain', id, name })
  }

  const addFolder = async () => {
    if (!canEdit) return
    const name = folderName.trim()
    if (!name) {
      showToast({ tone: 'error', text: 'Folder name is required.' })
      return
    }
    await folderCreateMutation.mutateAsync(name)
  }

  const saveFolder = async (folder: Folder) => {
    const name = (folderDrafts[folder.id] ?? '').trim()
    if (!name) {
      showToast({ tone: 'error', text: 'Folder name is required.' })
      return
    }
    if (name === folder.name) {
      showToast({ tone: 'info', text: 'Folder name is already up to date.' })
      return
    }
    await folderUpdateMutation.mutateAsync({ id: folder.id, name })
  }

  const removeFolder = async (folder: Folder) => {
    setConfirmState({ kind: 'folder', folder })
  }

  const activeFilterCount = [statusFilter !== 'all', sslExpiryWithin.trim(), domainExpiryWithin.trim(), ...Object.keys(metadataFilters)].filter(Boolean).length
  const hasActiveFilters = Boolean(search.trim() || statusFilter !== 'all' || selectedFolder !== 'all' || sslExpiryWithin.trim() || domainExpiryWithin.trim() || Object.keys(metadataFilters).length > 0)

  const canMoveCustom = canEdit && sortMode === 'custom' && search.trim() === '' && statusFilter === 'all' && selectedFolder === 'all' && !sslExpiryWithin && !domainExpiryWithin

  const handleMove = async (e: React.MouseEvent, id: number, direction: -1 | 1) => {
    e.stopPropagation()
    if (!canMoveCustom) return

    const allIDs = reorderSourceDomains
      .slice()
      .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0))
      .map(domain => domain.id)
    const index = allIDs.indexOf(id)
    const targetIndex = index + direction
    if (index < 0 || targetIndex < 0 || targetIndex >= allIDs.length) return

    const [moved] = allIDs.splice(index, 1)
    allIDs.splice(targetIndex, 0, moved)

    try {
      await saveDomainOrder(allIDs)
      invalidateDomainQueries(qc)
    } catch (err) {
      showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to reorder domains.') })
    }
  }

  const renderDomainCard = (d: Domain, idx: number) => {
    const tableMetadata = visibleMetadataSummary(d.metadata, visibleTableFields, 'table')
    const openDomain = () => navigate(`/domains/${d.id}`)

    return (
      <div
        key={d.id}
        className="card group cursor-pointer p-3 transition-all duration-200 hover:-translate-y-0.5 hover:border-blue-500/20 hover:shadow-[0_18px_40px_rgb(2_6_23/0.26)]"
        onClick={openDomain}
        onKeyDown={(event) => activateCardOnKey(event, openDomain)}
        role="button"
        tabIndex={0}
        aria-label={`Open domain details for ${d.name}`}
      >
        <div className="flex items-center gap-3">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium text-gray-100">{d.name}{d.port && d.port !== 443 ? `:${d.port}` : ''}</span>
              <StatusBadge status={d.last_check?.overall_status ?? 'unknown'} title={d.last_check?.primary_reason_text} />
              {hasAdvisoryNotes(d) && (
                <span className="rounded-full border border-slate-600/40 bg-slate-500/10 px-2 py-0.5 text-xs text-slate-300" title={d.last_check?.primary_reason_text}>
                  notes
                </span>
              )}
              {d.tags.length > 0 && (
                <div className="flex items-center gap-1 text-xs text-gray-500">
                  <Tag size={11} />
                  {d.tags.map(tag => (
                    <span key={tag.toLowerCase()} className="rounded-full border border-blue-500/20 bg-blue-500/10 px-2 py-0.5 text-blue-300">
                      {tag}
                    </span>
                  ))}
                </div>
              )}
              {d.check_mode === 'ssl_only' && (
                <span className="rounded-full bg-violet-500/10 px-2 py-0.5 text-xs text-violet-400">SSL Only</span>
              )}
              {d.custom_ca_pem?.trim() && (
                <span className="rounded-full bg-cyan-500/10 px-2 py-0.5 text-xs text-cyan-400">custom CA</span>
              )}
              {d.dns_servers?.trim() && (
                <span className="rounded-full bg-amber-500/10 px-2 py-0.5 text-xs text-amber-400">custom DNS</span>
              )}
              {!d.enabled && (
                <span className="rounded-full bg-gray-800 px-2 py-0.5 text-xs text-gray-600">disabled</span>
              )}
            </div>

            {d.last_check && (
              <div className="mt-0.5 text-xs text-gray-500">
                Last checked {formatDistanceToNow(new Date(d.last_check.checked_at), { addSuffix: true })}
                {d.last_check.ssl_version && ` | ${d.last_check.ssl_version}`}
                {d.last_check.ssl_issuer && ` | ${d.last_check.ssl_issuer}`}
                {secondaryReason(d) && ` | ${secondaryReason(d)}`}
                {tableMetadata.map(item => ` | ${item.label}=${item.value}`).join('')}
                {!tableMetadata.length && metadataSearchText(d.metadata) && ` | ${metadataSearchText(d.metadata)}`}
              </div>
            )}
            {!d.last_check && (
              <div className="mt-0.5 text-xs text-gray-500">
                {tagsToText(d.tags) || tableMetadata.map(item => `${item.label}=${item.value}`).join(' | ') || metadataSearchText(d.metadata) || 'No checks yet'}
              </div>
            )}
          </div>

          <div className="hidden w-48 space-y-1 xl:block">
            <ExpiryBar days={d.last_check?.ssl_expiry_days} label="SSL" warningDays={sslWarning} criticalDays={sslCritical} />
            {d.check_mode === 'ssl_only' ? (
              <div className="text-xs text-gray-600">Domain: N/A</div>
            ) : (
              <ExpiryBar days={d.last_check?.domain_expiry_days} label="Domain" warningDays={domainWarning} criticalDays={domainCritical} />
            )}
          </div>

          <div className="flex flex-shrink-0 items-center gap-1.5">
            {canMoveCustom && (
              <>
                <button
                  className="btn-ghost p-1.5"
                  onClick={e => handleMove(e, d.id, -1)}
                  disabled={safePage === 1 && idx === 0}
                  title="Move up"
                  aria-label="Move domain up"
                >
                  <ArrowUp size={14} />
                </button>
                <button
                  className="btn-ghost p-1.5"
                  onClick={e => handleMove(e, d.id, 1)}
                  disabled={safePage === totalPages && idx === domains.length - 1}
                  title="Move down"
                  aria-label="Move domain down"
                >
                  <ArrowDown size={14} />
                </button>
              </>
            )}

            {canEdit && (
              <button
                className="btn-ghost p-1.5"
                onClick={e => handleCheck(e, d.id)}
                disabled={checkingId === d.id}
                title="Run check now"
                aria-label="Run check now"
              >
                <RefreshCw size={14} className={checkingId === d.id ? 'animate-spin' : ''} />
              </button>
            )}

            <a
              href={`https://${d.name}${d.port && d.port !== 443 ? `:${d.port}` : ''}`}
              target="_blank"
              rel="noopener noreferrer"
              className="btn-ghost p-1.5"
              onClick={e => e.stopPropagation()}
              title="Open site"
              aria-label="Open site"
            >
              <ExternalLink size={14} />
            </a>

            {canEdit && (
              <button
                className="btn-danger p-1.5"
                onClick={e => handleDelete(e, d.id, d.name)}
                title="Delete"
                aria-label="Delete domain"
              >
                <Trash2 size={14} />
              </button>
            )}
            <ChevronRight size={15} className="text-gray-600 transition-colors group-hover:text-blue-300" />
          </div>
        </div>
      </div>
    )
  }

  if (isLoading && !pageData) {
    return (
      <div className="space-y-5 p-6">
        <PageHeadingSkeleton />
        <ListCardSkeleton count={6} />
      </div>
    )
  }

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-white">Domains</h1>
          <p className="text-sm text-gray-400 mt-0.5">{summary?.total ?? total} domain{(summary?.total ?? total) !== 1 ? 's' : ''} monitored</p>
        </div>
        <div className="flex items-center gap-2">
          {bootstrap?.features.csv_export && (
            <a className="btn-ghost border border-gray-700" href={exportDomainsCsvUrl(filters)}>
              <Download size={16} />
              Export CSV
            </a>
          )}
          {canEdit && (
            <button className="btn-primary" onClick={() => setShowAdd(true)}>
              <Plus size={16} />
              Add Domain
            </button>
          )}
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          className={`btn text-xs ${selectedFolder === 'all' ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
          onClick={() => setSelectedFolder('all')}
        >
          All ({summary?.total ?? total})
        </button>
        {folders.map(folder => (
          <button
            key={folder.id}
            className={`btn text-xs ${selectedFolder === String(folder.id) ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
            onClick={() => setSelectedFolder(String(folder.id))}
          >
            {folder.name} ({folder.domain_count || 0})
          </button>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-[220px] flex-1 max-w-xs">
          <input
            id="domains-search"
            ref={searchRef}
            className="input pr-24"
            placeholder="Search domains, tags, metadata..."
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <span className="shortcut-hint pointer-events-none absolute right-3 top-1/2 -translate-y-1/2">
            Ctrl/Cmd+K
          </span>
        </div>

        <select id="domains-sort" className="select w-auto min-w-[140px]" value={sortMode} onChange={e => setSortMode(e.target.value as SortMode)}>
          <option value="custom">Custom order</option>
          <option value="name">Alphabetical</option>
          <option value="status">By status</option>
          <option value="created_at">By added date</option>
          <option value="domain_expiry">Domain expiry</option>
          <option value="ssl_expiry">Cert expiry</option>
          <option value="last_check">Last check</option>
        </select>
        <select id="domains-sort-dir" className="select w-auto min-w-[110px]" value={sortDir} onChange={e => setSortDir(e.target.value as 'asc' | 'desc')}>
          <option value="asc">Asc</option>
          <option value="desc">Desc</option>
        </select>

        <button
          type="button"
          className={`btn-ghost border ${filtersOpen ? 'border-blue-500/40 bg-blue-500/10 text-blue-300' : 'border-gray-700'}`}
          onClick={() => setFiltersOpen(!filtersOpen)}
        >
          <SlidersHorizontal size={14} />
          Filters
          {activeFilterCount > 0 && (
            <span className="ml-0.5 rounded-full bg-blue-500 px-1.5 py-0.5 text-[10px] font-bold text-white leading-none">
              {activeFilterCount}
            </span>
          )}
          <ChevronDown size={14} className={`transition-transform ${filtersOpen ? 'rotate-180' : ''}`} />
        </button>

        {canEdit && (
          <button
            type="button"
            className={`btn-ghost border ${foldersOpen ? 'border-blue-500/40 bg-blue-500/10 text-blue-300' : 'border-gray-700'}`}
            onClick={() => setFoldersOpen(!foldersOpen)}
          >
            <Settings2 size={14} />
            Folders
            <ChevronDown size={14} className={`transition-transform ${foldersOpen ? 'rotate-180' : ''}`} />
          </button>
        )}

        {hasActiveFilters && (
          <button
            type="button"
            className="btn-ghost border border-gray-700 text-yellow-400"
            onClick={() => {
              setSearch('')
              setStatusFilter('all')
              setSelectedFolder('all')
              setSortMode('custom')
              setSortDir('asc')
              setSSLExpiryWithin('')
              setDomainExpiryWithin('')
              setMetadataFilters({})
              setPage(1)
            }}
          >
            Reset
          </button>
        )}
      </div>

      {filtersOpen && (
        <div className="page-transition rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-5">
            <div>
              <label className="label" htmlFor="domains-status-filter">Status</label>
              <select
                id="domains-status-filter"
                className="select"
                value={statusFilter}
                onChange={e => setStatusFilter(e.target.value)}
              >
                <option value="all">All</option>
                <option value="ok">OK</option>
                <option value="warning">Warning</option>
                <option value="critical">Critical</option>
                <option value="error">Error</option>
                <option value="unknown">Unknown</option>
              </select>
            </div>
            <div>
              <label className="label" htmlFor="ssl-expiry-within">SSL &le; days</label>
              <input
                id="ssl-expiry-within"
                className="input"
                type="number"
                min={0}
                placeholder="e.g. 30"
                value={sslExpiryWithin}
                onChange={e => setSSLExpiryWithin(e.target.value)}
              />
            </div>
            <div>
              <label className="label" htmlFor="domain-expiry-within">Domain &le; days</label>
              <input
                id="domain-expiry-within"
                className="input"
                type="number"
                min={0}
                placeholder="e.g. 90"
                value={domainExpiryWithin}
                onChange={e => setDomainExpiryWithin(e.target.value)}
              />
            </div>
            <div>
              <label className="label" htmlFor="domains-page-size">Per page</label>
              <select id="domains-page-size" className="select" value={pageSize} onChange={e => { setPageSize(Number(e.target.value)); setPage(1) }}>
                {[10, 20, 50, 100].map(size => (
                  <option key={size} value={size}>{size}</option>
                ))}
              </select>
            </div>
            </div>

            {filterableFields.length > 0 && (
              <div>
                <div className="mb-2 text-xs font-semibold text-slate-400">Inventory fields</div>
                <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
                  {filterableFields.map(field => (
                    <div key={field.key}>
                      <label className="label" htmlFor={`filter-${field.key}`}>{field.label}</label>
                      {field.type === 'select' ? (
                        <select
                          id={`filter-${field.key}`}
                          className="select"
                          value={metadataFilters[field.key] ?? ''}
                          onChange={e => setMetadataFilters(current => updateFilter(current, field.key, e.target.value))}
                        >
                          <option value="">All</option>
                          {(field.options ?? []).map(option => (
                            <option key={`${field.key}-${option.value}`} value={option.value}>{option.label}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          id={`filter-${field.key}`}
                          className="input"
                          type={field.type === 'date' ? 'date' : field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
                          value={metadataFilters[field.key] ?? ''}
                          onChange={e => setMetadataFilters(current => updateFilter(current, field.key, e.target.value))}
                          placeholder={field.placeholder || field.label}
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
        </div>
      )}

      {foldersOpen && canEdit && (
        <div className="page-transition rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-4">
            <div className="flex items-end gap-2">
            <div className="flex-1 max-w-xs">
              <label className="label" htmlFor="quick-folder-create">New folder</label>
              <input
                id="quick-folder-create"
                className="input"
                placeholder="backend, public-sites, staging"
                value={folderName}
                onChange={e => setFolderName(e.target.value)}
              />
            </div>
            <button className="btn-ghost border border-gray-700" onClick={addFolder} disabled={folderCreateMutation.isPending}>
              <FolderPlus size={14} />
              {folderCreateMutation.isPending ? 'Creating...' : 'Create'}
            </button>
            </div>

            {folders.length > 0 && (
              <div className="space-y-2">
                {folders.map(folder => (
                  <div key={folder.id} className="flex items-center gap-2">
                    <input
                      id={`folder-${folder.id}`}
                      className="input flex-1 max-w-xs"
                      value={folderDrafts[folder.id] ?? folder.name}
                      onChange={e => setFolderDrafts(drafts => ({ ...drafts, [folder.id]: e.target.value }))}
                    />
                    <span className="text-xs text-slate-500 w-12 text-right">{folder.domain_count}</span>
                    <button className="btn-ghost border border-slate-700 p-2" onClick={() => saveFolder(folder)} disabled={folderUpdateMutation.isPending} title="Save">
                      <Save size={13} />
                    </button>
                    <button className="btn-danger p-2" onClick={() => removeFolder(folder)} disabled={folderDeleteMutation.isPending} title="Delete">
                      <FolderX size={13} />
                    </button>
                  </div>
                ))}
              </div>
            )}
        </div>
      )}

      {!canEdit && (
        <div className="rounded-xl border border-blue-500/15 bg-blue-500/5 px-4 py-3 text-sm text-slate-300">
          Public or viewer mode is active. Editing controls are hidden, but domain status and expiry data remain fully visible.
        </div>
      )}

      {domains.length === 0 ? (
        <EmptyState
          icon={Plus}
          title={hasActiveFilters ? 'No domains match the current filters' : 'No domains in inventory yet'}
          description={hasActiveFilters
            ? 'Clear or relax the current filters to widen the inventory view.'
            : canEdit
              ? 'Add your first monitored endpoint to start building the inventory.'
              : 'The inventory is empty for your current view.'}
          action={canEdit && !hasActiveFilters ? (
            <button className="btn-primary" onClick={() => setShowAdd(true)}>
              <Plus size={14} />
              Add domain
            </button>
          ) : undefined}
        />
      ) : (
        <div className="space-y-2">
          {domains.length > 24 ? (
            <VirtualizedCardList
              items={domains}
              estimateSize={116}
              itemKey={(domain) => domain.id}
              renderItem={renderDomainCard}
            />
          ) : (
            domains.map((domain, index) => renderDomainCard(domain, index))
          )}

          <Pagination
            page={safePage}
            totalPages={totalPages}
            onPageChange={setPage}
            summary={`Showing ${total === 0 ? 0 : (safePage - 1) * pageSize + 1}-${Math.min(safePage * pageSize, total)} of ${total}`}
          />
        </div>
      )}

      {showAdd && canEdit && <AddDomainModal onClose={() => setShowAdd(false)} />}
      <ConfirmDialog
        open={confirmState?.kind === 'domain'}
        title="Delete domain"
        description={confirmState?.kind === 'domain' ? `Delete "${confirmState.name}" from monitoring? This removes its current record and history remains inaccessible from the UI.` : ''}
        confirmLabel="Delete domain"
        busy={deleteMutation.isPending}
        onClose={() => setConfirmState(null)}
        onConfirm={() => {
          if (confirmState?.kind !== 'domain') return
          deleteMutation.mutate(confirmState.id, {
            onSettled: () => setConfirmState(null),
          })
        }}
      />
      <ConfirmDialog
        open={confirmState?.kind === 'folder'}
        title="Delete folder"
        description={confirmState?.kind === 'folder' ? `Delete folder "${confirmState.folder.name}"? Assigned domains will be moved to "No folder".` : ''}
        confirmLabel="Delete folder"
        busy={folderDeleteMutation.isPending}
        onClose={() => setConfirmState(null)}
        onConfirm={() => {
          if (confirmState?.kind !== 'folder') return
          folderDeleteMutation.mutate(confirmState.folder.id, {
            onSettled: () => setConfirmState(null),
          })
        }}
      />
    </div>
  )
}

function invalidateDomainQueries(qc: QueryClient) {
  qc.invalidateQueries({ queryKey: ['domains'] })
  qc.invalidateQueries({ queryKey: ['domains-page'] })
  qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
  qc.invalidateQueries({ queryKey: ['domains-reorder'] })
  qc.invalidateQueries({ queryKey: ['summary'] })
  qc.invalidateQueries({ queryKey: ['folders'] })
  qc.invalidateQueries({ queryKey: ['tags'] })
}

function hasAdvisoryNotes(domain: Domain): boolean {
  return Boolean(
    domain.last_check?.overall_status === 'ok' &&
      domain.last_check?.status_reasons?.some(reason => reason.severity === 'advisory'),
  )
}

function secondaryReason(domain: Domain): string {
  const check = domain.last_check
  if (!check) return ''
  if (check.overall_status !== 'ok') {
    return check.primary_reason_text || ''
  }
  const note = check.status_reasons?.find(reason => reason.severity === 'advisory')
  if (!note) return ''
  return `validation note: ${note.detail || note.summary}`
}
