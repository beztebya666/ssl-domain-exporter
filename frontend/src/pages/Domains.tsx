import { useMemo, useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import {
  Plus,
  Trash2,
  RefreshCw,
  ExternalLink,
  Tag,
  ChevronRight,
  Download,
  ArrowUp,
  ArrowDown,
  FolderPlus,
} from 'lucide-react'
import {
  exportDomainsCsvUrl,
  fetchConfig,
  fetchDomains,
  deleteDomain,
  triggerCheck,
  fetchFolders,
  createFolder,
  reorderDomains,
} from '../api/client'
import StatusBadge from '../components/StatusBadge'
import ExpiryBar from '../components/ExpiryBar'
import AddDomainModal from '../components/AddDomainModal'
import { formatDistanceToNow } from 'date-fns'
import type { Domain } from '../types'
import { metadataSearchText, tagsToText } from '../lib/domainFields'

type SortMode = 'custom' | 'alphabetical' | 'status' | 'created_at' | 'domain_expiry' | 'ssl_expiry'

const STATUS_WEIGHT: Record<string, number> = {
  critical: 0,
  warning: 1,
  error: 2,
  ok: 3,
  unknown: 4,
}

export default function Domains() {
  const navigate = useNavigate()
  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [selectedFolder, setSelectedFolder] = useState<string>('all')
  const [sortMode, setSortMode] = useState<SortMode>('custom')
  const [checkingId, setCheckingId] = useState<number | null>(null)
  const [folderName, setFolderName] = useState('')

  const qc = useQueryClient()
  const { data: domains = [], isLoading } = useQuery({ queryKey: ['domains'], queryFn: fetchDomains })
  const { data: cfg } = useQuery({ queryKey: ['config'], queryFn: fetchConfig })
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })

  const deleteMutation = useMutation({
    mutationFn: deleteDomain,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domains'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
    },
  })

  const reorderMutation = useMutation({
    mutationFn: reorderDomains,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domains'] })
    },
  })

  const folderMutation = useMutation({
    mutationFn: createFolder,
    onSuccess: () => {
      setFolderName('')
      qc.invalidateQueries({ queryKey: ['folders'] })
    },
  })

  const handleCheck = async (e: React.MouseEvent, id: number) => {
    e.stopPropagation()
    setCheckingId(id)
    try {
      await triggerCheck(id)
      qc.invalidateQueries({ queryKey: ['domains'] })
    } finally {
      setCheckingId(null)
    }
  }

  const handleDelete = (e: React.MouseEvent, id: number, name: string) => {
    e.stopPropagation()
    if (!confirm(`Delete ${name}?`)) return
    deleteMutation.mutate(id)
  }

  const sortedByCustom = useMemo(() => {
    return [...domains].sort((a, b) => {
      if ((a.sort_order || 0) !== (b.sort_order || 0)) return (a.sort_order || 0) - (b.sort_order || 0)
      return a.name.localeCompare(b.name)
    })
  }, [domains])

  const filteredBase = useMemo(() => {
    return sortedByCustom.filter(d => {
      const searchText = search.toLowerCase()
      const matchSearch =
        d.name.toLowerCase().includes(searchText) ||
        tagsToText(d.tags).toLowerCase().includes(searchText) ||
        metadataSearchText(d.metadata).toLowerCase().includes(searchText)
      const matchStatus =
        statusFilter === 'all' ||
        d.last_check?.overall_status === statusFilter ||
        (!d.last_check && statusFilter === 'unknown')
      const matchFolder =
        selectedFolder === 'all' ||
        String(d.folder_id ?? '') === selectedFolder

      return matchSearch && matchStatus && matchFolder
    })
  }, [search, selectedFolder, sortedByCustom, statusFilter])

  const displayed = useMemo(() => sortDomains(filteredBase, sortMode), [filteredBase, sortMode])

  const handleMove = async (e: React.MouseEvent, id: number, direction: -1 | 1) => {
    e.stopPropagation()
    if (sortMode !== 'custom') return

    const visibleIDs = displayed.map(d => d.id)
    const currentIdx = visibleIDs.indexOf(id)
    const targetIdx = currentIdx + direction
    if (currentIdx < 0 || targetIdx < 0 || targetIdx >= visibleIDs.length) return

    const otherID = visibleIDs[targetIdx]
    const allIDs = sortedByCustom.map(d => d.id)
    const a = allIDs.indexOf(id)
    const b = allIDs.indexOf(otherID)
    if (a < 0 || b < 0) return

    ;[allIDs[a], allIDs[b]] = [allIDs[b], allIDs[a]]
    await reorderMutation.mutateAsync(allIDs)
  }

  const addFolder = async () => {
    const name = folderName.trim()
    if (!name) return
    await folderMutation.mutateAsync(name)
  }

  const folderCounts = useMemo(() => {
    const counts: Record<string, number> = { all: domains.length }
    for (const d of domains) {
      const key = String(d.folder_id ?? '')
      counts[key] = (counts[key] ?? 0) + 1
    }
    return counts
  }, [domains])

  return (
    <div className="p-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-white">Domains</h1>
          <p className="text-sm text-gray-400 mt-0.5">{domains.length} domain{domains.length !== 1 ? 's' : ''} monitored</p>
        </div>
        <div className="flex items-center gap-2">
          {cfg?.features.csv_export && (
            <a className="btn-ghost border border-gray-700" href={exportDomainsCsvUrl()}>
              <Download size={16} />
              Export CSV
            </a>
          )}
          <button className="btn-primary" onClick={() => setShowAdd(true)}>
            <Plus size={16} />
            Add Domain
          </button>
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        <button
          className={`btn text-xs ${selectedFolder === 'all' ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
          onClick={() => setSelectedFolder('all')}
        >
          All ({folderCounts.all || 0})
        </button>
        {folders.map(folder => (
          <button
            key={folder.id}
            className={`btn text-xs ${selectedFolder === String(folder.id) ? 'btn-primary' : 'btn-ghost border border-gray-700'}`}
            onClick={() => setSelectedFolder(String(folder.id))}
          >
            {folder.name} ({folderCounts[String(folder.id)] || 0})
          </button>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_200px_240px] gap-3 items-end">
        <div>
          <label className="label">Search</label>
          <input
            className="input"
            placeholder="Search domains, tags, or metadata..."
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>

        <div>
          <label className="label">Status filter</label>
          <select
            className="input"
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
          <label className="label">Sort</label>
          <select className="input" value={sortMode} onChange={e => setSortMode(e.target.value as SortMode)}>
            <option value="custom">Custom</option>
            <option value="alphabetical">Alphabetical</option>
            <option value="status">By status</option>
            <option value="created_at">By added date</option>
            <option value="domain_expiry">By domain expiry</option>
            <option value="ssl_expiry">By cert expiry</option>
          </select>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[1fr_auto] gap-2 items-end">
        <div>
          <label className="label">Quick folder create</label>
          <input
            className="input"
            placeholder="Enter folder name"
            value={folderName}
            onChange={e => setFolderName(e.target.value)}
          />
        </div>
        <button className="btn-ghost border border-gray-700" onClick={addFolder} disabled={folderMutation.isPending}>
          <FolderPlus size={14} />
          {folderMutation.isPending ? 'Creating...' : 'Create Folder'}
        </button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20 text-gray-500">
          <RefreshCw size={20} className="animate-spin mr-2" /> Loading...
        </div>
      ) : displayed.length === 0 ? (
        <div className="card text-center py-14 text-gray-500">
          <p className="text-lg mb-2">No domains found</p>
          <p className="text-sm">
            {domains.length === 0 ? 'Click "Add Domain" to get started' : 'Try adjusting filters or folder'}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {displayed.map((d, idx) => (
            <div
              key={d.id}
              className="card p-3 hover:border-gray-700 cursor-pointer transition-all group"
              onClick={() => navigate(`/domains/${d.id}`)}
            >
              <div className="flex items-center gap-3">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-medium text-gray-100">{d.name}{d.port && d.port !== 443 ? `:${d.port}` : ''}</span>
                    <StatusBadge status={d.last_check?.overall_status ?? 'unknown'} />
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
                      <span className="text-xs text-violet-400 bg-violet-500/10 px-2 py-0.5 rounded-full">SSL Only</span>
                    )}
                    {d.custom_ca_pem?.trim() && (
                      <span className="text-xs text-cyan-400 bg-cyan-500/10 px-2 py-0.5 rounded-full">custom CA</span>
                    )}
                    {d.dns_servers?.trim() && (
                      <span className="text-xs text-amber-400 bg-amber-500/10 px-2 py-0.5 rounded-full">custom DNS</span>
                    )}
                    {!d.enabled && (
                      <span className="text-xs text-gray-600 bg-gray-800 px-2 py-0.5 rounded-full">disabled</span>
                    )}
                  </div>

                  {d.last_check && (
                    <div className="text-xs text-gray-500 mt-0.5">
                      Last checked {formatDistanceToNow(new Date(d.last_check.checked_at), { addSuffix: true })}
                      {d.last_check.ssl_version && ` | ${d.last_check.ssl_version}`}
                      {d.last_check.ssl_issuer && ` | ${d.last_check.ssl_issuer}`}
                      {metadataSearchText(d.metadata) && ` | ${metadataSearchText(d.metadata)}`}
                    </div>
                  )}
                </div>

                <div className="w-48 space-y-1 hidden xl:block">
                  <ExpiryBar days={d.last_check?.ssl_expiry_days} label="SSL" />
                  {d.check_mode === 'ssl_only' ? (
                    <div className="text-xs text-gray-600">Domain: N/A</div>
                  ) : (
                    <ExpiryBar days={d.last_check?.domain_expiry_days} label="Domain" />
                  )}
                </div>

                <div className="flex items-center gap-1.5 flex-shrink-0">
                  {sortMode === 'custom' && (
                    <>
                      <button
                        className="btn-ghost p-1.5"
                        onClick={e => handleMove(e, d.id, -1)}
                        disabled={idx === 0 || reorderMutation.isPending}
                        title="Move up"
                      >
                        <ArrowUp size={14} />
                      </button>
                      <button
                        className="btn-ghost p-1.5"
                        onClick={e => handleMove(e, d.id, 1)}
                        disabled={idx === displayed.length - 1 || reorderMutation.isPending}
                        title="Move down"
                      >
                        <ArrowDown size={14} />
                      </button>
                    </>
                  )}

                  <button
                    className="btn-ghost p-1.5"
                    onClick={e => handleCheck(e, d.id)}
                    disabled={checkingId === d.id}
                    title="Run check now"
                  >
                    <RefreshCw size={14} className={checkingId === d.id ? 'animate-spin' : ''} />
                  </button>

                  <a
                    href={`https://${d.name}${d.port && d.port !== 443 ? `:${d.port}` : ''}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="btn-ghost p-1.5"
                    onClick={e => e.stopPropagation()}
                    title="Open site"
                  >
                    <ExternalLink size={14} />
                  </a>

                  <button
                    className="btn-danger p-1.5"
                    onClick={e => handleDelete(e, d.id, d.name)}
                    title="Delete"
                  >
                    <Trash2 size={14} />
                  </button>
                  <ChevronRight size={15} className="text-gray-600 group-hover:text-gray-400 transition-colors" />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {showAdd && <AddDomainModal onClose={() => setShowAdd(false)} />}
    </div>
  )
}

function sortDomains(domains: Domain[], mode: SortMode): Domain[] {
  const items = [...domains]

  switch (mode) {
    case 'alphabetical':
      return items.sort((a, b) => a.name.localeCompare(b.name))
    case 'status':
      return items.sort((a, b) => {
        const aWeight = STATUS_WEIGHT[a.last_check?.overall_status ?? 'unknown'] ?? 99
        const bWeight = STATUS_WEIGHT[b.last_check?.overall_status ?? 'unknown'] ?? 99
        if (aWeight !== bWeight) return aWeight - bWeight
        return a.name.localeCompare(b.name)
      })
    case 'created_at':
      return items.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    case 'domain_expiry':
      return items.sort((a, b) => compareNullableNumber(a.last_check?.domain_expiry_days, b.last_check?.domain_expiry_days, a.name, b.name))
    case 'ssl_expiry':
      return items.sort((a, b) => compareNullableNumber(a.last_check?.ssl_expiry_days, b.last_check?.ssl_expiry_days, a.name, b.name))
    case 'custom':
    default:
      return items.sort((a, b) => {
        if ((a.sort_order || 0) !== (b.sort_order || 0)) return (a.sort_order || 0) - (b.sort_order || 0)
        return a.name.localeCompare(b.name)
      })
  }
}

function compareNullableNumber(a: number | null | undefined, b: number | null | undefined, aName: string, bName: string): number {
  const aIsNil = a == null
  const bIsNil = b == null
  if (aIsNil && bIsNil) return aName.localeCompare(bName)
  if (aIsNil) return 1
  if (bIsNil) return -1
  if (a !== b) return a - b
  return aName.localeCompare(bName)
}
