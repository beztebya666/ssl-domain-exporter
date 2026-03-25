import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CalendarClock, Globe, Shield } from 'lucide-react'
import { fetchTimeline } from '../api/client'
import CollapsiblePanel from '../components/CollapsiblePanel'
import EmptyState from '../components/EmptyState'
import Pagination from '../components/Pagination'
import { ListCardSkeleton, PageHeadingSkeleton, StatCardSkeleton } from '../components/Skeleton'
import type { BootstrapConfig, TimelineEntry } from '../types'
import { formatDays } from '../lib/formatDays'
import { activateCardOnKey } from '../lib/utils'

type TimelineProps = {
  bootstrap?: BootstrapConfig
}

export default function Timeline({ bootstrap }: TimelineProps) {
  const navigate = useNavigate()
  const sslWarning = bootstrap?.alerts.ssl_expiry_warning_days ?? 30
  const sslCritical = bootstrap?.alerts.ssl_expiry_critical_days ?? 7
  const domainWarning = bootstrap?.alerts.domain_expiry_warning_days ?? 30
  const domainCritical = bootstrap?.alerts.domain_expiry_critical_days ?? 7

  const [sslPage, setSSLPage] = useState(1)
  const [domainPage, setDomainPage] = useState(1)

  const timelineQuery = useQuery({
    queryKey: ['timeline', sslPage, domainPage],
    queryFn: () => fetchTimeline({ ssl_page: sslPage, ssl_page_size: 20, domain_page: domainPage, domain_page_size: 20 }),
    placeholderData: previous => previous,
  })

  const timeline = timelineQuery.data

  if (bootstrap && !bootstrap.features.timeline_view) {
    return (
      <div className="p-6">
        <EmptyState
          icon={CalendarClock}
          title="Timeline view is disabled"
          description="Enable the timeline feature in Administration to review expiry pressure across SSL certificates and domain registrations."
        />
      </div>
    )
  }

  if (timelineQuery.isLoading && !timeline) {
    return (
      <div className="space-y-6 p-6">
        <PageHeadingSkeleton />
        <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
          {Array.from({ length: 3 }).map((_, index) => (
            <StatCardSkeleton key={index} />
          ))}
        </div>
        <div className="space-y-4">
          <ListCardSkeleton count={4} />
          <ListCardSkeleton count={4} />
        </div>
      </div>
    )
  }

  if (!timeline || (timeline.ssl.total === 0 && timeline.domain.total === 0)) {
    return (
      <div className="p-6">
        <EmptyState
          icon={CalendarClock}
          title="No expiry history yet"
          description="Run checks first to populate SSL and domain registration expiry data for the timeline."
        />
      </div>
    )
  }

  const summary = timeline.summary

  return (
    <div className="space-y-6 p-6">
      <div>
        <h1 className="text-xl font-bold text-white">Timeline</h1>
        <p className="mt-0.5 text-sm text-gray-400">
          Expiry pressure view for SSL certificates and domain registrations.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
        <TimelineStatCard title="SSL critical" value={summary.ssl_critical} hint={`<= ${sslCritical} days`} icon={AlertTriangle} tone="critical" />
        <TimelineStatCard title="SSL warning" value={summary.ssl_warning} hint={`${sslCritical + 1}-${sslWarning} days`} icon={Shield} tone="warning" />
        <TimelineStatCard title="Domain critical" value={summary.domain_critical} hint={`<= ${domainCritical} days`} icon={AlertTriangle} tone="critical" />
        <TimelineStatCard title="Domain warning" value={summary.domain_warning} hint={`${domainCritical + 1}-${domainWarning} days`} icon={Globe} tone="warning" />
      </div>

      <CollapsiblePanel
        title="SSL certificate timeline"
        description="Sorted from the most urgent SSL expiry to the least urgent."
        icon={Shield}
        defaultOpen
        bodyClassName="space-y-3"
      >
        <TimelineList
          items={timeline.ssl.items}
          warningDays={sslWarning}
          criticalDays={sslCritical}
          onOpen={(domainId) => navigate(`/domains/${domainId}`)}
        />
        <Pagination
          page={timeline.ssl.page}
          totalPages={Math.max(1, timeline.ssl.total_pages)}
          onPageChange={setSSLPage}
          summary={`Showing ${timeline.ssl.total === 0 ? 0 : (timeline.ssl.page - 1) * timeline.ssl.page_size + 1}-${Math.min(timeline.ssl.page * timeline.ssl.page_size, timeline.ssl.total)} of ${timeline.ssl.total} SSL records`}
        />
      </CollapsiblePanel>

      <CollapsiblePanel
        title="Domain registration timeline"
        description="Only domains with registration checks enabled are shown here."
        icon={Globe}
        defaultOpen
        bodyClassName="space-y-3"
      >
        {timeline.domain.total === 0 ? (
          <EmptyState
            icon={Globe}
            title="No domain registration records"
            description="Either registration checks are disabled for this inventory or no domain expiry data has been collected yet."
          />
        ) : (
          <>
            <TimelineList
              items={timeline.domain.items}
              warningDays={domainWarning}
              criticalDays={domainCritical}
              onOpen={(domainId) => navigate(`/domains/${domainId}`)}
            />
            <Pagination
              page={timeline.domain.page}
              totalPages={Math.max(1, timeline.domain.total_pages)}
              onPageChange={setDomainPage}
              summary={`Showing ${timeline.domain.total === 0 ? 0 : (timeline.domain.page - 1) * timeline.domain.page_size + 1}-${Math.min(timeline.domain.page * timeline.domain.page_size, timeline.domain.total)} of ${timeline.domain.total} domain records`}
            />
          </>
        )}
      </CollapsiblePanel>
    </div>
  )
}

function TimelineStatCard({
  title,
  value,
  hint,
  icon: Icon,
  tone,
}: {
  title: string
  value: number
  hint: string
  icon: typeof AlertTriangle
  tone: 'warning' | 'critical'
}) {
  const toneClass = tone === 'critical'
    ? 'bg-rose-500/10 text-rose-300 border-rose-500/20'
    : 'bg-amber-500/10 text-amber-300 border-amber-500/20'

  return (
    <div className="card flex items-center gap-4">
      <div className={`rounded-xl border p-3 ${toneClass}`}>
        <Icon size={20} />
      </div>
      <div className="min-w-0">
        <div className="text-2xl font-bold text-white">{value}</div>
        <div className="text-sm text-slate-300">{title}</div>
        <div className="text-xs text-slate-500">{hint}</div>
      </div>
    </div>
  )
}

function TimelineList({
  items,
  warningDays,
  criticalDays,
  onOpen,
}: {
  items: TimelineEntry[]
  warningDays: number
  criticalDays: number
  onOpen: (domainId: number) => void
}) {
  const maxDays = Math.max(warningDays, ...items.map(item => Math.max(item.days, 0)))

  return (
    <div className="space-y-3">
      {items.map((item) => {
        const pct = item.days <= 0 ? 100 : Math.max(4, Math.min(100, ((maxDays - item.days) / Math.max(maxDays, 1)) * 100))
        const toneClass = item.days <= criticalDays
          ? 'bg-rose-500'
          : item.days <= warningDays
            ? 'bg-amber-500'
            : 'bg-emerald-500'

        return (
          <div
            key={`${item.domain_id}-${item.kind}`}
            className="rounded-xl border border-slate-800 bg-slate-950/40 p-4 transition-all duration-150 hover:-translate-y-0.5 hover:shadow-lg hover:shadow-slate-950/20 cursor-pointer"
            onClick={() => onOpen(item.domain_id)}
            onKeyDown={(event) => activateCardOnKey(event, () => onOpen(item.domain_id))}
            role="button"
            tabIndex={0}
            aria-label={`Open ${item.name}`}
          >
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <div className="truncate text-sm font-semibold text-white">{item.name}</div>
                  <span className="rounded-full border border-slate-700 bg-slate-900 px-2 py-0.5 text-[11px] uppercase tracking-wide text-slate-300">
                    {item.kind}
                  </span>
                  <span className={`rounded-full px-2 py-0.5 text-[11px] ${item.days <= criticalDays ? 'bg-rose-500/10 text-rose-300' : item.days <= warningDays ? 'bg-amber-500/10 text-amber-300' : 'bg-emerald-500/10 text-emerald-300'}`}>
                    {formatDays(item.days)}
                  </span>
                </div>
                <div className="mt-1 text-xs text-slate-500">
                  {item.kind === 'ssl'
                    ? item.issuer ? `Issued by ${item.issuer}` : 'SSL certificate expiry data'
                    : 'Domain registration expiry data'}
                </div>
              </div>
              <div className="w-28 flex-shrink-0 text-right text-xs text-slate-500">
                {item.days <= 0 ? 'Immediate action' : `${item.days} day${item.days === 1 ? '' : 's'} remaining`}
              </div>
            </div>
            <div className="mt-3 h-2 overflow-hidden rounded-full bg-slate-800">
              <div className={`h-full rounded-full ${toneClass}`} style={{ width: `${pct}%` }} />
            </div>
          </div>
        )
      })}
    </div>
  )
}
