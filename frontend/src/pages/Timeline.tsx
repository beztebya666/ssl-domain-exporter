import { useQuery } from '@tanstack/react-query'
import { CalendarClock, Globe } from 'lucide-react'
import { fetchConfig, fetchDomains } from '../api/client'

interface TimelineItem {
  id: number
  name: string
  kind: 'ssl' | 'domain'
  days: number
}

export default function Timeline() {
  const { data: domains = [], isLoading } = useQuery({ queryKey: ['domains'], queryFn: fetchDomains })
  const { data: cfg } = useQuery({ queryKey: ['config'], queryFn: fetchConfig })

  if (cfg && !cfg.features.timeline_view) {
    return (
      <div className="p-6">
        <div className="card text-gray-400">Timeline view is disabled in settings.</div>
      </div>
    )
  }

  const items: TimelineItem[] = []
  for (const d of domains) {
    if (d.last_check?.ssl_expiry_days != null) {
      items.push({ id: d.id, name: d.name, kind: 'ssl', days: d.last_check.ssl_expiry_days })
    }
    if (d.last_check?.domain_expiry_days != null) {
      items.push({ id: d.id, name: d.name, kind: 'domain', days: d.last_check.domain_expiry_days })
    }
  }

  items.sort((a, b) => a.days - b.days)

  const maxDays = Math.max(30, ...items.map(i => i.days))

  return (
    <div className="p-6 space-y-5">
      <div>
        <h1 className="text-xl font-bold text-white">Timeline</h1>
        <p className="text-sm text-gray-400 mt-0.5">Expiry timeline for SSL and domain dates</p>
      </div>

      {isLoading ? (
        <div className="card text-gray-500">Loading timeline...</div>
      ) : items.length === 0 ? (
        <div className="card text-center py-12 text-gray-500">
          <Globe size={32} className="mx-auto mb-2 opacity-30" />
          No expiry data yet.
        </div>
      ) : (
        <div className="card">
          <div className="space-y-3">
            {items.map((item, idx) => {
              const pct = Math.max(2, Math.min(100, (item.days / maxDays) * 100))
              const color = item.days <= 7 ? 'bg-red-500' : item.days <= 30 ? 'bg-yellow-500' : 'bg-green-500'
              return (
                <div key={`${item.id}-${item.kind}-${idx}`} className="space-y-1">
                  <div className="flex items-center justify-between text-sm">
                    <div className="text-gray-200 truncate pr-2">
                      {item.name} <span className="text-xs text-gray-500">({item.kind})</span>
                    </div>
                    <div className="text-xs text-gray-400 flex items-center gap-1.5">
                      <CalendarClock size={12} /> {item.days}d
                    </div>
                  </div>
                  <div className="h-2 bg-gray-800 rounded-full overflow-hidden">
                    <div className={`h-full ${color}`} style={{ width: `${pct}%` }} />
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
