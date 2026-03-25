import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import type { LucideIcon } from 'lucide-react'
import {
  AlertTriangle,
  ArrowDownUp,
  BadgeInfo,
  BellRing,
  CheckCircle,
  ChevronDown,
  ChevronUp,
  Clock,
  Download,
  Globe,
  Radio,
  Shield,
  SlidersHorizontal,
  XCircle,
} from 'lucide-react'
import { formatDistanceToNow } from 'date-fns'
import { exportDomainsCsvUrl, fetchCustomFields, fetchDomainsPage, fetchFolders, fetchSummary, fetchTags } from '../api/client'
import EmptyState from '../components/EmptyState'
import Pagination from '../components/Pagination'
import { ListCardSkeleton, PageHeadingSkeleton, StatCardSkeleton, TableSkeleton } from '../components/Skeleton'
import StatusBadge from '../components/StatusBadge'
import ExpiryBar from '../components/ExpiryBar'
import type { AuthMe, BootstrapConfig, Domain } from '../types'
import { filterableCustomFields, visibleMetadataSummary } from '../lib/customFields'
import { formatDays } from '../lib/formatDays'
import { useDebouncedValue } from '../lib/useDebouncedValue'
import { activateCardOnKey, isTypingTarget, parseOptionalInt, updateFilter } from '../lib/utils'

function SummaryCard({ label, value, icon: Icon, color, total }: {
  label: string
  value: number
  icon: LucideIcon
  color: string
  total: number
}) {
  const normalizedTotal = Math.max(total, 1)
  const progress = Math.min(1, value / normalizedTotal)
  const circumference = 2 * Math.PI * 22
  const dashOffset = circumference * (1 - progress)

  return (
    <div className="card flex items-center gap-4">
      <div className="relative flex-shrink-0">
        <svg width="58" height="58" viewBox="0 0 58 58" className="drop-shadow-sm">
          <circle cx="29" cy="29" r="22" stroke="var(--ring-track)" strokeWidth="6" fill="none" />
          <circle
            cx="29"
            cy="29"
            r="22"
            className={`stroke-current ${color}`}
            strokeWidth="6"
            fill="none"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={dashOffset}
            transform="rotate(-90 29 29)"
          />
        </svg>
        <div className={`absolute inset-0 flex items-center justify-center rounded-full ${color}`}>
          <Icon size={18} />
        </div>
      </div>
      <div>
        <div className="text-2xl font-bold text-white">{value}</div>
        <div className="text-sm text-gray-400">{label}</div>
      </div>
    </div>
  )
}

type DashboardProps = {
  me?: AuthMe
  bootstrap?: BootstrapConfig
}

type SortKey = 'name' | 'status' | 'ssl_expiry' | 'domain_expiry' | 'last_check'
type SortDir = 'asc' | 'desc'

type ExpiryWatchItem = {
  domain: Domain
  kind: 'SSL' | 'Domain'
  days: number
  severity: 'warning' | 'critical'
}

export default function Dashboard({ bootstrap }: DashboardProps) {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchRef = useRef<HTMLInputElement | null>(null)
  const [selectedTag, setSelectedTag] = useState('all')
  const [selectedFolder, setSelectedFolder] = useState('all')
  const [search, setSearch] = useState('')
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [sslExpiryWithin, setSSLExpiryWithin] = useState('')
  const [domainExpiryWithin, setDomainExpiryWithin] = useState('')
  const [metadataFilters, setMetadataFilters] = useState<Record<string, string>>({})
  const [sortKey, setSortKey] = useState<SortKey>('status')
  const [sortDir, setSortDir] = useState<SortDir>('asc')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [live, setLive] = useState(false)
  const debouncedSearch = useDebouncedValue(search, 250)

  const filters = useMemo(() => ({
    search: debouncedSearch.trim() || undefined,
    tag: selectedTag !== 'all' ? selectedTag : undefined,
    folder_id: selectedFolder !== 'all' ? Number(selectedFolder) : undefined,
    metadata_filters: Object.keys(metadataFilters).length > 0 ? metadataFilters : undefined,
    ssl_expiry_lte: parseOptionalInt(sslExpiryWithin),
    domain_expiry_lte: parseOptionalInt(domainExpiryWithin),
    sort_by: sortKey,
    sort_dir: sortDir,
    page,
    page_size: pageSize,
  }), [debouncedSearch, domainExpiryWithin, metadataFilters, page, pageSize, selectedFolder, selectedTag, sortDir, sortKey, sslExpiryWithin])

  const { data: pageData, isLoading } = useQuery({
    queryKey: ['dashboard-domains-page', filters],
    queryFn: () => fetchDomainsPage(filters),
    placeholderData: previous => previous,
    refetchInterval: live ? 30000 : false,
  })
  const { data: summary } = useQuery({ queryKey: ['summary'], queryFn: fetchSummary, refetchInterval: live ? 30000 : false })
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })
  const { data: allTags = [] } = useQuery({
    queryKey: ['tags'],
    queryFn: fetchTags,
    enabled: Boolean(bootstrap?.features.dashboard_tag_filter),
  })
  const { data: customFields = [] } = useQuery({ queryKey: ['custom-fields'], queryFn: () => fetchCustomFields(false) })

  const domains = pageData?.items ?? []
  const total = pageData?.total ?? 0
  const totalPages = Math.max(1, pageData?.total_pages ?? 1)
  const safePage = Math.min(page, totalPages)
  const filterableFields = filterableCustomFields(customFields)
  const visibleTableFields = customFields.filter(field => field.enabled && field.visible_in_table)

  const alertsConfig = bootstrap?.alerts
  const sslWarning = alertsConfig?.ssl_expiry_warning_days ?? 14
  const sslCritical = alertsConfig?.ssl_expiry_critical_days ?? 3
  const domainWarning = alertsConfig?.domain_expiry_warning_days ?? 30
  const domainCritical = alertsConfig?.domain_expiry_critical_days ?? 7

  const currentPageSummary = useMemo(() => domains.reduce((acc, domain) => {
    const status = domain.last_check?.overall_status ?? 'unknown'
    acc.total += 1
    if (status in acc) {
      acc[status as keyof typeof acc] += 1
    }
    return acc
  }, { total: 0, ok: 0, warning: 0, critical: 0, error: 0, unknown: 0 }), [domains])

  const hasActiveFilters = Boolean(
    search.trim() ||
      selectedTag !== 'all' ||
      selectedFolder !== 'all' ||
      Object.keys(metadataFilters).length > 0 ||
      sslExpiryWithin.trim() ||
      domainExpiryWithin.trim(),
  )
  const summaryData = hasActiveFilters ? currentPageSummary : summary
  const totalForCards = summaryData?.total ?? 0

  useEffect(() => {
    if (!pageData) return

    const prefetch = (targetPage: number) => {
      if (targetPage < 1 || targetPage > totalPages || targetPage === safePage) return
      const nextFilters = { ...filters, page: targetPage }
      queryClient.prefetchQuery({
        queryKey: ['dashboard-domains-page', nextFilters],
        queryFn: () => fetchDomainsPage(nextFilters),
      })
    }

    prefetch(safePage + 1)
    prefetch(safePage - 1)
  }, [filters, pageData, queryClient, safePage, totalPages])

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
      }
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const operationalAlerts = domains.filter(domain => {
    const status = domain.last_check?.overall_status
    return status === 'warning' || status === 'critical' || status === 'error'
  })

  const validationFindings = domains.filter(hasAdvisoryFindings)

  const expiryWatchlist = useMemo<ExpiryWatchItem[]>(() => {
    const items: ExpiryWatchItem[] = []
    for (const domain of domains) {
      const check = domain.last_check
      if (!check) continue
      if (check.ssl_expiry_days != null && check.ssl_expiry_days <= sslWarning) {
        items.push({
          domain,
          kind: 'SSL',
          days: check.ssl_expiry_days,
          severity: check.ssl_expiry_days <= sslCritical ? 'critical' : 'warning',
        })
      }
      if (!check.registration_check_skipped && check.domain_expiry_days != null && check.domain_expiry_days <= domainWarning) {
        items.push({
          domain,
          kind: 'Domain',
          days: check.domain_expiry_days,
          severity: check.domain_expiry_days <= domainCritical ? 'critical' : 'warning',
        })
      }
    }
    return items.sort((a, b) => {
      if (a.days !== b.days) return a.days - b.days
      if (a.kind !== b.kind) return a.kind.localeCompare(b.kind)
      return a.domain.name.localeCompare(b.domain.name)
    })
  }, [domainCritical, domainWarning, domains, sslCritical, sslWarning])

  const setSorting = (nextKey: SortKey) => {
    setPage(1)
    if (sortKey === nextKey) {
      setSortDir(prev => (prev === 'asc' ? 'desc' : 'asc'))
      return
    }
    setSortKey(nextKey)
    setSortDir('asc')
  }

  if (isLoading && !pageData) {
    return (
      <div className="space-y-6 p-6">
        <PageHeadingSkeleton />
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <StatCardSkeleton key={index} />
          ))}
        </div>
        <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <ListCardSkeleton count={3} />
          <ListCardSkeleton count={3} />
        </div>
        <TableSkeleton rows={6} columns={6} />
      </div>
    )
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold text-white">Dashboard</h1>
          <p className="mt-0.5 text-sm text-gray-400">
            Operational alerts, expiry visibility, and validation findings.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-[220px]">
            <input
              id="dashboard-search"
              ref={searchRef}
              className="input pr-24"
              value={search}
              onChange={e => {
                setSearch(e.target.value)
                setPage(1)
              }}
              placeholder="Search domains..."
            />
            <span className="shortcut-hint pointer-events-none absolute right-3 top-1/2 -translate-y-1/2">
              Ctrl/Cmd+K
            </span>
          </div>
          <button
            type="button"
            className={`btn-ghost border ${live ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300' : 'border-gray-700'}`}
            onClick={() => setLive(current => !current)}
            aria-pressed={live}
          >
            <Radio size={14} />
            {live ? 'Live' : 'Live Off'}
          </button>
          <button
            type="button"
            className={`btn-ghost border ${filtersOpen ? 'border-blue-500/40 bg-blue-500/10 text-blue-300' : 'border-gray-700'}`}
            onClick={() => setFiltersOpen(!filtersOpen)}
          >
            <SlidersHorizontal size={14} />
            Filters
            {hasActiveFilters && (
              <span className="ml-0.5 rounded-full bg-blue-500 px-1.5 py-0.5 text-[10px] font-bold text-white leading-none">
                {[selectedTag !== 'all', selectedFolder !== 'all', sslExpiryWithin.trim(), domainExpiryWithin.trim(), ...Object.keys(metadataFilters)].filter(Boolean).length}
              </span>
            )}
            <ChevronDown size={14} className={`transition-transform ${filtersOpen ? 'rotate-180' : ''}`} />
          </button>
          <a className="btn-ghost border border-gray-700" href={exportDomainsCsvUrl(filters)}>
            <Download size={14} />
            CSV
          </a>
          {hasActiveFilters && (
            <button
              type="button"
              className="btn-ghost border border-gray-700 text-yellow-400"
              onClick={() => {
                setSelectedTag('all')
                setSelectedFolder('all')
                setSearch('')
                setSSLExpiryWithin('')
                setDomainExpiryWithin('')
                setMetadataFilters({})
                setSortKey('status')
                setSortDir('asc')
                setPage(1)
              }}
            >
              Reset
            </button>
          )}
        </div>
      </div>

      {filtersOpen && (
        <div className="page-transition rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-4">
            <div className="grid grid-cols-2 gap-3 md:grid-cols-4 lg:grid-cols-6">
            {bootstrap?.features.dashboard_tag_filter && (
              <div>
                <label className="label" htmlFor="dashboard-tag-filter">Tag</label>
                <select
                  id="dashboard-tag-filter"
                  className="select"
                  value={selectedTag}
                  onChange={e => {
                    setSelectedTag(e.target.value)
                    setPage(1)
                  }}
                >
                  <option value="all">All tags</option>
                  {allTags.map(tag => (
                    <option key={tag} value={tag}>{tag}</option>
                  ))}
                </select>
              </div>
            )}

            <div>
              <label className="label" htmlFor="dashboard-folder-filter">Folder</label>
              <select
                id="dashboard-folder-filter"
                className="select"
                value={selectedFolder}
                onChange={e => {
                  setSelectedFolder(e.target.value)
                  setPage(1)
                }}
              >
                <option value="all">All folders</option>
                {folders.map(folder => (
                  <option key={folder.id} value={folder.id}>{folder.name}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="label" htmlFor="dashboard-ssl-within">SSL &le; days</label>
              <input
                id="dashboard-ssl-within"
                className="input"
                type="number"
                min={0}
                value={sslExpiryWithin}
                onChange={e => {
                  setSSLExpiryWithin(e.target.value)
                  setPage(1)
                }}
              />
            </div>

            <div>
              <label className="label" htmlFor="dashboard-domain-within">Domain &le; days</label>
              <input
                id="dashboard-domain-within"
                className="input"
                type="number"
                min={0}
                value={domainExpiryWithin}
                onChange={e => {
                  setDomainExpiryWithin(e.target.value)
                  setPage(1)
                }}
              />
            </div>

            <div>
              <label className="label" htmlFor="dashboard-page-size">Per page</label>
              <select
                id="dashboard-page-size"
                className="select"
                value={pageSize}
                onChange={e => {
                  setPageSize(Number(e.target.value))
                  setPage(1)
                }}
              >
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
                      <label className="label" htmlFor={`dashboard-filter-${field.key}`}>{field.label}</label>
                      {field.type === 'select' ? (
                        <select
                          id={`dashboard-filter-${field.key}`}
                          className="select"
                          value={metadataFilters[field.key] ?? ''}
                          onChange={e => {
                            setMetadataFilters(current => updateFilter(current, field.key, e.target.value))
                            setPage(1)
                          }}
                        >
                          <option value="">All</option>
                          {field.options.map(option => (
                            <option key={`${field.key}-${option.value}`} value={option.value}>{option.label}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          id={`dashboard-filter-${field.key}`}
                          className="input"
                          type={field.type === 'date' ? 'date' : field.type === 'email' ? 'email' : field.type === 'url' ? 'url' : 'text'}
                          value={metadataFilters[field.key] ?? ''}
                          onChange={e => {
                            setMetadataFilters(current => updateFilter(current, field.key, e.target.value))
                            setPage(1)
                          }}
                          placeholder={field.placeholder || field.label}
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {hasActiveFilters && (
              <p className="text-xs text-slate-500">
                Summary cards reflect the current page while filters are active; export uses the full filtered result set.
              </p>
            )}
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <SummaryCard label={hasActiveFilters ? 'Current Page' : 'Total Domains'} value={summaryData?.total ?? 0} icon={Globe} color="text-blue-400" total={Math.max(summaryData?.total ?? 0, 1)} />
        <SummaryCard label="Healthy" value={summaryData?.ok ?? 0} icon={CheckCircle} color="text-green-400" total={totalForCards} />
        <SummaryCard label="Warnings" value={summaryData?.warning ?? 0} icon={AlertTriangle} color="text-yellow-400" total={totalForCards} />
        <SummaryCard label="Critical" value={(summaryData?.critical ?? 0) + (summaryData?.error ?? 0)} icon={XCircle} color="text-red-400" total={totalForCards} />
      </div>

      {operationalAlerts.length > 0 && (
        <div className="card space-y-3">
          <h2 className="flex items-center gap-2 font-semibold text-white">
            <BellRing size={16} className="text-yellow-400" />
            Operational Alerts
            <span className="text-xs font-normal text-slate-500">{operationalAlerts.length}</span>
          </h2>
          <p className="text-xs text-slate-500">
            Findings that currently affect service risk or the main badge policy in this result set.
          </p>
          {operationalAlerts.slice(0, 12).map(domain => (
            <div
              key={domain.id}
              className="flex cursor-pointer items-start justify-between gap-4 rounded-lg bg-gray-800 p-3 transition-colors hover:bg-gray-750"
              onClick={() => navigate(`/domains/${domain.id}`)}
              onKeyDown={(event) => activateCardOnKey(event, () => navigate(`/domains/${domain.id}`))}
              role="button"
              tabIndex={0}
              aria-label={`Open operational alert details for ${domain.name}`}
            >
              <div className="flex min-w-0 items-start gap-3">
                <StatusBadge status={domain.last_check?.overall_status ?? 'unknown'} title={primaryReason(domain)} />
                <div className="min-w-0">
                  <div className="text-sm font-medium text-gray-200">{domain.name}</div>
                  <div className="mt-1 text-xs text-slate-400">{primaryReason(domain) || 'Alert reason unavailable'}</div>
                </div>
              </div>
              <div className="w-36 flex-shrink-0 space-y-1">
                {domain.last_check?.ssl_expiry_days != null && (
                  <ExpiryBar days={domain.last_check.ssl_expiry_days} label="SSL" warningDays={sslWarning} criticalDays={sslCritical} />
                )}
                {domain.last_check && !domain.last_check.registration_check_skipped && domain.last_check.domain_expiry_days != null && (
                  <ExpiryBar days={domain.last_check.domain_expiry_days} label="Domain" warningDays={domainWarning} criticalDays={domainCritical} />
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {expiryWatchlist.length > 0 && (
        <div className="card space-y-3">
          <h2 className="flex items-center gap-2 font-semibold text-white">
            <Shield size={16} className="text-blue-400" />
            Expiry Watchlist
            <span className="text-xs font-normal text-slate-500">{expiryWatchlist.length}</span>
          </h2>
          <p className="text-xs text-slate-500">
            SSL certificates and domain registrations currently inside the configured thresholds.
          </p>
          <div className="space-y-3">
            {expiryWatchlist.slice(0, 12).map(item => (
              <div
                key={`${item.domain.id}-${item.kind}`}
                className="flex cursor-pointer items-center gap-4 rounded-lg bg-gray-800 p-3 hover:bg-gray-750"
                onClick={() => navigate(`/domains/${item.domain.id}`)}
                onKeyDown={(event) => activateCardOnKey(event, () => navigate(`/domains/${item.domain.id}`))}
                role="button"
                tabIndex={0}
                aria-label={`Open ${item.kind.toLowerCase()} expiry details for ${item.domain.name}`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <div className="truncate text-sm font-medium text-gray-200">{item.domain.name}</div>
                    <span className={`rounded-full border px-2 py-0.5 text-[11px] ${item.kind === 'SSL' ? 'border-blue-500/30 bg-blue-500/10 text-blue-300' : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300'}`}>
                      {item.kind}
                    </span>
                    <StatusBadge status={item.severity} title={`${item.kind} expiry is within the configured threshold`} />
                  </div>
                  <div className="mt-0.5 text-xs text-gray-500">
                    {item.kind === 'SSL'
                      ? item.domain.last_check?.ssl_issuer
                        ? `Issued by ${item.domain.last_check.ssl_issuer}`
                        : 'SSL certificate is approaching the configured threshold'
                      : 'Domain registration expiry is approaching the configured threshold'}
                  </div>
                </div>
                <div className="w-44 flex-shrink-0">
                  <ExpiryBar
                    days={item.days}
                    label={item.kind}
                    warningDays={item.kind === 'SSL' ? sslWarning : domainWarning}
                    criticalDays={item.kind === 'SSL' ? sslCritical : domainCritical}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {validationFindings.length > 0 && (
        <div className="card space-y-3">
          <h2 className="flex items-center gap-2 font-semibold text-white">
            <BadgeInfo size={16} className="text-slate-300" />
            Validation Findings
            <span className="text-xs font-normal text-slate-500">{validationFindings.length}</span>
          </h2>
          <p className="text-xs text-slate-500">
            Advisory-only findings remain visible for investigation but do not currently raise the main operational badge.
          </p>
          <div className="space-y-3">
            {validationFindings.slice(0, 12).map(domain => (
              <div
                key={domain.id}
                className="flex cursor-pointer items-start justify-between gap-4 rounded-lg bg-gray-800 p-3 transition-colors hover:bg-gray-750"
                onClick={() => navigate(`/domains/${domain.id}`)}
                onKeyDown={(event) => activateCardOnKey(event, () => navigate(`/domains/${domain.id}`))}
                role="button"
                tabIndex={0}
                aria-label={`Open validation notes for ${domain.name}`}
              >
                <div className="flex min-w-0 items-start gap-3">
                  <StatusBadge status="ok" title={advisorySummary(domain)} />
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <div className="text-sm font-medium text-gray-200">{domain.name}</div>
                      <span className="rounded-full border border-slate-600/40 bg-slate-500/10 px-2 py-0.5 text-[11px] text-slate-300">
                        notes
                      </span>
                    </div>
                    <div className="mt-1 text-xs text-slate-400">{advisorySummary(domain)}</div>
                  </div>
                </div>
                <div className="whitespace-nowrap text-right text-xs text-gray-500">
                  {domain.last_check?.checked_at ? formatDistanceToNow(new Date(domain.last_check.checked_at), { addSuffix: true }) : '-'}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="card">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <h2 className="flex items-center gap-2 font-semibold text-white">
            <Globe size={16} className="text-gray-400" />
            All Domains
          </h2>
          <div className="text-xs text-slate-500">
            Showing {total === 0 ? 0 : (safePage - 1) * pageSize + 1}-{Math.min(safePage * pageSize, total)} of {total}
          </div>
        </div>

        {domains.length === 0 ? (
          <EmptyState
            icon={Globe}
            title={hasActiveFilters ? 'No matching domains' : 'No monitored domains yet'}
            description={hasActiveFilters
              ? 'Adjust the current filters or clear them to broaden the inventory view.'
              : 'Add domains in the inventory page to start building your operational dashboard.'}
          />
        ) : (
          <>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-800 text-left text-gray-500">
                    <SortableHeader label="Domain" active={sortKey === 'name'} direction={sortDir} onClick={() => setSorting('name')} />
                    <SortableHeader label="Status" active={sortKey === 'status'} direction={sortDir} onClick={() => setSorting('status')} />
                    <SortableHeader label="SSL Expiry" active={sortKey === 'ssl_expiry'} direction={sortDir} onClick={() => setSorting('ssl_expiry')} />
                    <SortableHeader label="Domain Expiry" active={sortKey === 'domain_expiry'} direction={sortDir} onClick={() => setSorting('domain_expiry')} />
                    <SortableHeader label="Last Check" active={sortKey === 'last_check'} direction={sortDir} onClick={() => setSorting('last_check')} />
                    <th className="pb-3 font-medium">Reason</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-800/50">
                  {domains.map(domain => (
                    <tr
                      key={domain.id}
                      className="cursor-pointer transition-colors hover:bg-gray-800/50"
                      onClick={() => navigate(`/domains/${domain.id}`)}
                      onKeyDown={(event) => activateCardOnKey(event, () => navigate(`/domains/${domain.id}`))}
                      role="button"
                      tabIndex={0}
                      aria-label={`Open domain details for ${domain.name}`}
                    >
                      <td className="py-3">
                        <div className="font-medium text-gray-200">{domain.name}</div>
                        {visibleMetadataSummary(domain.metadata, visibleTableFields, 'table').length > 0 && (
                          <div className="mt-1 text-xs text-slate-500">
                            {visibleMetadataSummary(domain.metadata, visibleTableFields, 'table').map(item => `${item.label}=${item.value}`).join(' | ')}
                          </div>
                        )}
                      </td>
                      <td className="py-3">
                        <StatusBadge status={domain.last_check?.overall_status ?? 'unknown'} title={primaryReason(domain)} />
                      </td>
                      <td className="py-3">
                        {domain.last_check?.ssl_expiry_days != null ? (
                          <span className={expiryColorClass(domain.last_check.ssl_expiry_days, sslWarning, sslCritical)}>
                            {formatDays(domain.last_check.ssl_expiry_days)}
                          </span>
                        ) : (
                          <span className="text-gray-600">-</span>
                        )}
                      </td>
                      <td className="py-3">
                        {domain.last_check?.registration_check_skipped ? (
                          <span className="text-gray-600">N/A</span>
                        ) : domain.last_check?.domain_expiry_days != null ? (
                          <span className={expiryColorClass(domain.last_check.domain_expiry_days, domainWarning, domainCritical)}>
                            {formatDays(domain.last_check.domain_expiry_days)}
                          </span>
                        ) : (
                          <span className="text-gray-600">-</span>
                        )}
                      </td>
                      <td className="py-3 text-gray-500">
                        {domain.last_check ? (
                          <span className="flex items-center gap-1.5">
                            <Clock size={12} />
                            {formatDistanceToNow(new Date(domain.last_check.checked_at), { addSuffix: true })}
                          </span>
                        ) : '-'}
                      </td>
                      <td className="max-w-[28rem] py-3 text-xs text-slate-400">
                        {primaryReason(domain) || advisorySummary(domain) || <span className="text-gray-600">No issues</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <Pagination
              page={safePage}
              totalPages={totalPages}
              onPageChange={setPage}
              summary={`Sorted by ${sortKey.replace('_', ' ')} (${sortDir})`}
            />
          </>
        )}
      </div>
    </div>
  )
}

function SortableHeader({ label, active, direction, onClick }: { label: string; active: boolean; direction: SortDir; onClick: () => void }) {
  return (
    <th className="pb-3 font-medium">
      <button
        className={`inline-flex items-center gap-1 transition-colors ${active ? 'text-blue-300' : 'text-gray-400 hover:text-gray-200'}`}
        onClick={onClick}
        aria-pressed={active}
      >
        {label}
        {active ? (direction === 'asc' ? <ChevronUp size={12} /> : <ChevronDown size={12} />) : <ArrowDownUp size={12} />}
      </button>
    </th>
  )
}

function primaryReason(domain: Domain): string {
  const check = domain.last_check
  if (!check) return ''
  if (check.overall_status === 'ok') return ''
  if (check.primary_reason_text) return check.primary_reason_text
  const actionable = check.status_reasons?.find(reason => reason.severity !== 'advisory')
  if (actionable) {
    return actionable.detail || actionable.summary
  }
  return ''
}

function advisorySummary(domain: Domain): string {
  const check = domain.last_check
  if (!check?.status_reasons?.length) return ''
  const notes = check.status_reasons
    .filter(reason => reason.severity === 'advisory')
    .map(reason => reason.detail || reason.summary)
    .filter(Boolean)
  if (notes.length === 0) return ''
  if (notes.length === 1) return notes[0]
  return `${notes[0]} (+${notes.length - 1} more)`
}

function expiryColorClass(days: number, warningDays: number, criticalDays: number): string {
  if (days <= criticalDays) return 'text-red-400'
  if (days <= warningDays) return 'text-yellow-400'
  return 'text-green-400'
}

function hasAdvisoryFindings(domain: Domain): boolean {
  return Boolean(
    domain.last_check?.overall_status === 'ok' &&
      domain.last_check.status_reasons?.some(reason => reason.severity === 'advisory'),
  )
}
