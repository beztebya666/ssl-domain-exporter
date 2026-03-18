import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import type { LucideIcon } from 'lucide-react'
import { AlertTriangle, CheckCircle, XCircle, Globe, Shield, Clock, RefreshCw } from 'lucide-react'
import { fetchConfig, fetchDomains, fetchSummary } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import ExpiryBar from '../components/ExpiryBar'
import { formatDistanceToNow } from 'date-fns'

function SummaryCard({ label, value, icon: Icon, color }: {
  label: string; value: number; icon: LucideIcon; color: string
}) {
  return (
    <div className="card flex items-center gap-4">
      <div className={`p-3 rounded-xl ${color}`}>
        <Icon size={22} />
      </div>
      <div>
        <div className="text-2xl font-bold text-white">{value}</div>
        <div className="text-sm text-gray-400">{label}</div>
      </div>
    </div>
  )
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [selectedTag, setSelectedTag] = useState<string>('all')
  const { data: domains = [], isLoading } = useQuery({ queryKey: ['domains'], queryFn: fetchDomains })
  const { data: summary } = useQuery({ queryKey: ['summary'], queryFn: fetchSummary })
  const { data: cfg } = useQuery({ queryKey: ['config'], queryFn: fetchConfig })

  const allTags = Array.from(new Set(
    domains
      .flatMap(d => d.tags.split(',').map(t => t.trim()).filter(Boolean))
  )).sort((a, b) => a.localeCompare(b))

  const filteredDomains = selectedTag === 'all'
    ? domains
    : domains.filter(d => d.tags.split(',').map(t => t.trim()).includes(selectedTag))

  const criticalDomains = filteredDomains.filter(d => d.last_check?.overall_status === 'critical')
  const warningDomains = filteredDomains.filter(d => d.last_check?.overall_status === 'warning')
  const soonExpiring = filteredDomains
    .filter(d => d.last_check?.ssl_expiry_days != null && d.last_check.ssl_expiry_days! <= 30)
    .sort((a, b) => (a.last_check?.ssl_expiry_days ?? 9999) - (b.last_check?.ssl_expiry_days ?? 9999))

  const visibleSummary = filteredDomains.reduce((acc, d) => {
    const status = d.last_check?.overall_status ?? 'unknown'
    acc.total += 1
    if (status in acc) {
      acc[status as keyof typeof acc] += 1
    }
    return acc
  }, { total: 0, ok: 0, warning: 0, critical: 0, error: 0, unknown: 0 })

  const summaryData = cfg?.features.dashboard_tag_filter && selectedTag !== 'all'
    ? visibleSummary
    : summary

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-white">Dashboard</h1>
        <p className="text-sm text-gray-400 mt-0.5">Overview of all monitored domains</p>
      </div>

      {cfg?.features.dashboard_tag_filter && (
        <div className="max-w-xs">
          <label className="label">Tag filter</label>
          <select className="input" value={selectedTag} onChange={e => setSelectedTag(e.target.value)}>
            <option value="all">All tags</option>
            {allTags.map(tag => (
              <option key={tag} value={tag}>{tag}</option>
            ))}
          </select>
        </div>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard label="Total Domains" value={summaryData?.total ?? 0} icon={Globe} color="bg-blue-500/15 text-blue-400" />
        <SummaryCard label="Healthy" value={summaryData?.ok ?? 0} icon={CheckCircle} color="bg-green-500/15 text-green-400" />
        <SummaryCard label="Warnings" value={(summaryData?.warning ?? 0)} icon={AlertTriangle} color="bg-yellow-500/15 text-yellow-400" />
        <SummaryCard label="Critical" value={(summaryData?.critical ?? 0) + (summaryData?.error ?? 0)} icon={XCircle} color="bg-red-500/15 text-red-400" />
      </div>

      {/* Critical & Warning alerts */}
      {(criticalDomains.length > 0 || warningDomains.length > 0) && (
        <div className="card space-y-3">
          <h2 className="font-semibold text-white flex items-center gap-2">
            <AlertTriangle size={16} className="text-yellow-400" /> Alerts
          </h2>
          {[...criticalDomains, ...warningDomains].map(d => (
            <div
              key={d.id}
              className="flex items-center justify-between p-3 bg-gray-800 rounded-lg cursor-pointer hover:bg-gray-750 transition-colors"
              onClick={() => navigate(`/domains/${d.id}`)}
            >
              <div className="flex items-center gap-3">
                <StatusBadge status={d.last_check?.overall_status ?? 'unknown'} />
                <span className="font-medium text-sm text-gray-200">{d.name}</span>
              </div>
              <div className="text-right text-xs text-gray-500">
                {d.last_check?.ssl_expiry_days != null && (
                  <span>SSL: {d.last_check.ssl_expiry_days}d</span>
                )}
                {d.last_check?.domain_expiry_days != null && (
                  <span className="ml-3">Domain: {d.last_check.domain_expiry_days}d</span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* SSL expiring soon */}
      {soonExpiring.length > 0 && (
        <div className="card space-y-3">
          <h2 className="font-semibold text-white flex items-center gap-2">
            <Shield size={16} className="text-blue-400" /> SSL Certificates Expiring Soon
          </h2>
          <div className="space-y-3">
            {soonExpiring.slice(0, 10).map(d => (
              <div
                key={d.id}
                className="flex items-center gap-4 p-3 bg-gray-800 rounded-lg cursor-pointer hover:bg-gray-750"
                onClick={() => navigate(`/domains/${d.id}`)}
              >
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-gray-200 truncate">{d.name}</div>
                  <div className="text-xs text-gray-500 mt-0.5">
                    {d.last_check?.ssl_issuer && `Issued by ${d.last_check.ssl_issuer}`}
                  </div>
                </div>
                <div className="w-40 flex-shrink-0">
                  <ExpiryBar days={d.last_check?.ssl_expiry_days} label="SSL" />
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* All domains table */}
      <div className="card">
        <h2 className="font-semibold text-white mb-4 flex items-center gap-2">
          <Globe size={16} className="text-gray-400" /> All Domains
        </h2>
        {isLoading ? (
          <div className="flex items-center justify-center py-10 text-gray-500">
            <RefreshCw size={18} className="animate-spin mr-2" /> Loading...
          </div>
        ) : filteredDomains.length === 0 ? (
          <div className="text-center py-10 text-gray-500">
            <Globe size={32} className="mx-auto mb-3 opacity-30" />
            <p>No matching domains.</p>
            <p className="text-sm mt-1">Adjust tag filter or add domains in <a href="/domains" className="text-blue-400 hover:underline">Domains</a>.</p>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-gray-500 border-b border-gray-800">
                  <th className="pb-3 font-medium">Domain</th>
                  <th className="pb-3 font-medium">Status</th>
                  <th className="pb-3 font-medium">SSL Expiry</th>
                  <th className="pb-3 font-medium">Domain Expiry</th>
                  <th className="pb-3 font-medium">Last Check</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {filteredDomains.map(d => (
                  <tr
                    key={d.id}
                    className="hover:bg-gray-800/50 cursor-pointer transition-colors"
                    onClick={() => navigate(`/domains/${d.id}`)}
                  >
                    <td className="py-3 font-medium text-gray-200">{d.name}</td>
                    <td className="py-3">
                      <StatusBadge status={d.last_check?.overall_status ?? 'unknown'} />
                    </td>
                    <td className="py-3">
                      {d.last_check?.ssl_expiry_days != null ? (
                        <span className={
                          d.last_check.ssl_expiry_days <= 7 ? 'text-red-400' :
                          d.last_check.ssl_expiry_days <= 30 ? 'text-yellow-400' : 'text-green-400'
                        }>
                          {d.last_check.ssl_expiry_days}d
                        </span>
                      ) : <span className="text-gray-600">-</span>}
                    </td>
                    <td className="py-3">
                      {d.last_check?.domain_expiry_days != null ? (
                        <span className={
                          d.last_check.domain_expiry_days <= 7 ? 'text-red-400' :
                          d.last_check.domain_expiry_days <= 30 ? 'text-yellow-400' : 'text-green-400'
                        }>
                          {d.last_check.domain_expiry_days}d
                        </span>
                      ) : <span className="text-gray-600">-</span>}
                    </td>
                    <td className="py-3 text-gray-500">
                      {d.last_check ? (
                        <span className="flex items-center gap-1.5">
                          <Clock size={12} />
                          {formatDistanceToNow(new Date(d.last_check.checked_at), { addSuffix: true })}
                        </span>
                      ) : '-'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
