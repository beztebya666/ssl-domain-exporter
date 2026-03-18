import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft, RefreshCw, Shield, Globe, Link2, Clock,
  CheckCircle, XCircle, AlertTriangle, ExternalLink, ChevronDown, ChevronUp, Pencil, Check, X, Server
} from 'lucide-react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend
} from 'recharts'
import { fetchDomain, fetchHistory, triggerCheck, updateDomain, fetchFolders } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import { format, formatDistanceToNow } from 'date-fns'
import type { Check as CheckType, ChainCert } from '../types'

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

const CHART_COLORS = {
  ssl: '#3b82f6',
  domain: '#10b981',
}

export default function DomainDetail() {
  const { id } = useParams<{ id: string }>()
  const domainId = Number(id)
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [checking, setChecking] = useState(false)
  const [editing, setEditing] = useState(false)
  const [editName, setEditName] = useState('')
  const [editTags, setEditTags] = useState('')
  const [editEnabled, setEditEnabled] = useState(true)
  const [editInterval, setEditInterval] = useState(21600)
  const [editPort, setEditPort] = useState(443)
  const [editFolderValue, setEditFolderValue] = useState('')
  const [editCustomCAPEM, setEditCustomCAPEM] = useState('')

  const { data: domain, isLoading } = useQuery({
    queryKey: ['domain', domainId],
    queryFn: () => fetchDomain(domainId),
    enabled: !!domainId,
  })

  const { data: history = [] } = useQuery({
    queryKey: ['history', domainId],
    queryFn: () => fetchHistory(domainId, 100),
    enabled: !!domainId,
  })
  const { data: folders = [] } = useQuery({ queryKey: ['folders'], queryFn: fetchFolders })

  const updateMutation = useMutation({
    mutationFn: (data: Parameters<typeof updateDomain>[1]) => updateDomain(domainId, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['domain', domainId] })
      qc.invalidateQueries({ queryKey: ['domains'] })
      setEditing(false)
    },
  })

  const startEdit = () => {
    if (!domain) return
    setEditName(domain.name)
    setEditTags(domain.tags)
    setEditEnabled(domain.enabled)
    setEditInterval(domain.check_interval)
    setEditPort(domain.port || 443)
    setEditFolderValue(domain.folder_id ? String(domain.folder_id) : '')
    setEditCustomCAPEM(domain.custom_ca_pem ?? '')
    setEditing(true)
  }

  const handleCheck = async () => {
    setChecking(true)
    try {
      await triggerCheck(domainId)
      qc.invalidateQueries({ queryKey: ['domain', domainId] })
      qc.invalidateQueries({ queryKey: ['history', domainId] })
      qc.invalidateQueries({ queryKey: ['domains'] })
    } finally {
      setChecking(false)
    }
  }

  // Build chart data from history (reversed, oldest first)
  const chartData = [...history].reverse().map(c => ({
    time: format(new Date(c.checked_at), 'MM/dd HH:mm'),
    ssl: c.ssl_expiry_days,
    domain: c.domain_expiry_days,
  }))

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500">
        <RefreshCw className="animate-spin mr-2" size={20} /> Loading...
      </div>
    )
  }

  if (!domain) {
    return (
      <div className="p-6">
        <p className="text-gray-400">Domain not found.</p>
        <button className="btn-ghost mt-3" onClick={() => navigate('/domains')}>
          <ArrowLeft size={14} /> Back
        </button>
      </div>
    )
  }

  const check = domain.last_check

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <button className="btn-ghost p-2" onClick={() => navigate('/domains')}>
            <ArrowLeft size={16} />
          </button>
          {editing ? (
            <div className="flex items-center gap-2">
              <input
                className="input text-lg font-bold py-1 w-64"
                value={editName}
                onChange={e => setEditName(e.target.value)}
              />
              <button
                className="btn-primary py-1"
                onClick={() => updateMutation.mutate({
                  name: editName, tags: editTags,
                  enabled: editEnabled, check_interval: editInterval,
                  port: editPort,
                  folder_id: editFolderValue ? Number(editFolderValue) : 0,
                  custom_ca_pem: editCustomCAPEM
                })}
              >
                <Check size={14} /> Save
              </button>
              <button className="btn-ghost py-1" onClick={() => setEditing(false)}>
                <X size={14} />
              </button>
            </div>
          ) : (
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-bold text-white">{domain.name}</h1>
              <StatusBadge status={check?.overall_status ?? 'unknown'} />
              <button className="btn-ghost p-1.5 opacity-60 hover:opacity-100" onClick={startEdit}>
                <Pencil size={13} />
              </button>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2">
          <a href={`https://${domain.name}${domain.port && domain.port !== 443 ? `:${domain.port}` : ''}`} target="_blank" rel="noopener noreferrer" className="btn-ghost">
            <ExternalLink size={14} /> Open
          </a>
          <button className="btn-primary" onClick={handleCheck} disabled={checking}>
            <RefreshCw size={14} className={checking ? 'animate-spin' : ''} />
            {checking ? 'Checking...' : 'Check Now'}
          </button>
        </div>
      </div>

      {/* Edit extra fields */}
      {editing && (
        <div className="card grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <label className="label">Tags</label>
            <input className="input" value={editTags} onChange={e => setEditTags(e.target.value)} />
          </div>
          <div>
            <label className="label">Check interval (seconds)</label>
            <input className="input" type="number" value={editInterval} onChange={e => setEditInterval(Number(e.target.value))} />
          </div>
          <div>
            <label className="label">Enabled</label>
            <select className="input" value={editEnabled ? '1' : '0'} onChange={e => setEditEnabled(e.target.value === '1')}>
              <option value="1">Yes</option>
              <option value="0">No</option>
            </select>
          </div>
          <div>
            <label className="label">HTTPS Port</label>
            <input className="input" type="number" min={1} max={65535} value={editPort} onChange={e => setEditPort(Math.max(1, Math.min(65535, Number(e.target.value) || 443)))} />
          </div>
          <div>
            <label className="label">Folder</label>
            <select className="input" value={editFolderValue} onChange={e => setEditFolderValue(e.target.value)}>
              <option value="">No folder</option>
              {folders.map(folder => (
                <option key={folder.id} value={folder.id}>{folder.name}</option>
              ))}
            </select>
          </div>
          <div className="md:col-span-4">
            <label className="label">Custom Root CA (PEM)</label>
            <textarea
              className="input h-32 resize-y font-mono text-xs"
              value={editCustomCAPEM}
              onChange={e => setEditCustomCAPEM(e.target.value)}
              placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
            />
          </div>
        </div>
      )}

      {/* Info panels */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
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

        {/* Domain Panel */}
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Globe size={15} className="text-green-400" /> Domain Registration
          </h3>
          {check?.domain_check_error ? (
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
      </div>

      {/* Chart */}
      {chartData.length > 1 && (
        <div className="card">
          <h3 className="font-semibold text-white mb-4 flex items-center gap-2">
            <Clock size={15} className="text-gray-400" /> Expiry History
          </h3>
          <ResponsiveContainer width="100%" height={220}>
            <LineChart data={chartData} margin={{ top: 5, right: 20, left: 0, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#1f2937" />
              <XAxis dataKey="time" tick={{ fill: '#6b7280', fontSize: 11 }} />
              <YAxis tick={{ fill: '#6b7280', fontSize: 11 }} unit="d" />
              <Tooltip
                contentStyle={{ backgroundColor: '#111827', border: '1px solid #374151', borderRadius: 8 }}
                labelStyle={{ color: '#9ca3af' }}
                itemStyle={{ color: '#e5e7eb' }}
              />
              <Legend wrapperStyle={{ color: '#9ca3af', fontSize: 12 }} />
              <Line
                type="monotone" dataKey="ssl" name="SSL expiry (days)"
                stroke={CHART_COLORS.ssl} strokeWidth={2} dot={false} connectNulls
              />
              <Line
                type="monotone" dataKey="domain" name="Domain expiry (days)"
                stroke={CHART_COLORS.domain} strokeWidth={2} dot={false} connectNulls
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}

      {/* HTTP and advanced security checks */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <div className="card space-y-0">
          <h3 className="font-semibold text-white mb-3 flex items-center gap-2">
            <Server size={15} className="text-blue-400" /> HTTP/HTTPS
          </h3>
          {check ? (
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
          {check ? (
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
      {history.length > 0 && (
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
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-800/50">
                {history.map(c => (
                  <tr key={c.id} className="hover:bg-gray-800/30">
                    <td className="py-2 text-gray-400">
                      {format(new Date(c.checked_at), 'yyyy-MM-dd HH:mm')}
                    </td>
                    <td className="py-2">
                      <StatusBadge status={c.overall_status} />
                    </td>
                    <td className="py-2 text-gray-300">
                      {c.ssl_expiry_days != null ? `${c.ssl_expiry_days}d` : '-'}
                    </td>
                    <td className="py-2 text-gray-300">
                      {c.domain_expiry_days != null ? `${c.domain_expiry_days}d` : '-'}
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
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
