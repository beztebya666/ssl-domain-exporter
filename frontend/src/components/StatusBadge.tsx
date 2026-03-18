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

export default function StatusBadge({ status }: { status: Status }) {
  return (
    <span className={clsx('inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-medium border', styles[status])}>
      <span className={clsx('w-1.5 h-1.5 rounded-full', dots[status])} />
      {status}
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
