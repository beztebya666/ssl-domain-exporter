import { useState } from 'react'
import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { ChevronDown } from 'lucide-react'
import clsx from 'clsx'

type Props = {
  title: string
  description?: ReactNode
  icon?: LucideIcon
  defaultOpen?: boolean
  children: ReactNode
  className?: string
  bodyClassName?: string
}

export default function CollapsiblePanel({
  title,
  description,
  icon: Icon,
  defaultOpen = false,
  children,
  className,
  bodyClassName,
}: Props) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className={clsx('rounded-2xl border border-slate-800 bg-slate-900/40', className)}>
      <button
        type="button"
        className="flex w-full items-start justify-between gap-4 px-4 py-4 text-left"
        onClick={() => setOpen(current => !current)}
        aria-expanded={open}
      >
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm font-semibold text-white">
            {Icon && <Icon size={16} className="text-blue-400" />}
            {title}
          </div>
          {description && <div className="mt-1 text-xs text-slate-500">{description}</div>}
        </div>
        <ChevronDown size={16} className={clsx('mt-0.5 flex-shrink-0 text-slate-400 transition-transform duration-300', open && 'rotate-180')} />
      </button>

      {open && (
        <div className="page-transition">
          <div className="overflow-hidden">
            <div className={clsx('border-t border-slate-800 px-4 py-4', bodyClassName)}>
              {children}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
