import clsx from 'clsx'

interface ExpiryBarProps {
  days: number | null | undefined
  label: string
  warningDays?: number
  criticalDays?: number
}

export default function ExpiryBar({ days, label, warningDays = 30, criticalDays = 7 }: ExpiryBarProps) {
  if (days == null) {
    return (
      <div>
        <div className="flex justify-between text-xs text-gray-500 mb-1">
          <span>{label}</span>
          <span>unknown</span>
        </div>
        <div className="h-1.5 bg-gray-800 rounded-full" />
      </div>
    )
  }

  const maxDays = 365
  const pct = Math.min(Math.max((days / maxDays) * 100, 0), 100)

  const color = days <= criticalDays
    ? 'bg-red-500'
    : days <= warningDays
    ? 'bg-yellow-500'
    : 'bg-green-500'

  return (
    <div>
      <div className="flex justify-between text-xs mb-1">
        <span className="text-gray-400">{label}</span>
        <span className={clsx(
          'font-medium',
          days <= criticalDays ? 'text-red-400' : days <= warningDays ? 'text-yellow-400' : 'text-green-400'
        )}>
          {days < 0 ? `Expired ${Math.abs(days)}d ago` : `${days}d`}
        </span>
      </div>
      <div className="h-1.5 bg-gray-800 rounded-full overflow-hidden">
        <div
          className={clsx('h-full rounded-full transition-all', color)}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  )
}
