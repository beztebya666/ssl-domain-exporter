import { useId } from 'react'
import clsx from 'clsx'

type Status = 'ok' | 'warning' | 'critical' | 'error' | 'unknown'

const styles: Record<Status, string> = {
  ok: 'bg-green-500/15 text-green-400 border-green-500/30',
  warning: 'bg-yellow-500/15 text-yellow-400 border-yellow-500/30',
  critical: 'bg-red-500/15 text-red-400 border-red-500/30',
  error: 'bg-red-900/30 text-red-300 border-red-700/30',
  unknown: 'bg-gray-700/30 text-gray-400 border-gray-600/30',
}

const dots: Record<Status, string> = {
  ok: 'bg-green-400',
  warning: 'bg-yellow-400',
  critical: 'bg-red-400',
  error: 'bg-red-300',
  unknown: 'bg-gray-400',
}

export default function StatusBadge({ status, title }: { status: Status; title?: string }) {
  const tooltipId = useId()

  return (
    <span
      className={clsx('relative inline-flex', title ? 'group/status-badge' : undefined)}
      tabIndex={title ? 0 : -1}
      aria-label={title ? `${status}: ${title}` : status}
      aria-describedby={title ? tooltipId : undefined}
    >
      <span
        className={clsx(
          'inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium',
          title ? 'cursor-help' : undefined,
          styles[status],
        )}
      >
        <span className={clsx('h-1.5 w-1.5 rounded-full', dots[status])} />
        {status}
      </span>
      {title && (
        <span
          id={tooltipId}
          role="tooltip"
          className="status-badge-tooltip pointer-events-none absolute left-1/2 top-[calc(100%+0.45rem)] z-20 hidden w-64 -translate-x-1/2 rounded-xl border border-slate-700 bg-slate-950/95 px-3 py-2 text-left text-[11px] font-normal leading-5 text-slate-200 shadow-2xl group-hover/status-badge:block group-focus-within/status-badge:block"
        >
          {title}
        </span>
      )}
    </span>
  )
}

export function StatusDot({ status, size = 10 }: { status: Status; size?: number }) {
  const colors: Record<Status, string> = {
    ok: 'bg-green-400',
    warning: 'bg-yellow-400',
    critical: 'bg-red-400',
    error: 'bg-red-300',
    unknown: 'bg-gray-500',
  }
  return (
    <span
      className={clsx('inline-block rounded-full flex-shrink-0', colors[status])}
      style={{ width: size, height: size }}
    />
  )
}
