import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Info, X } from 'lucide-react'
import clsx from 'clsx'

type ToastTone = 'success' | 'error' | 'info'

type ToastInput = {
  tone: ToastTone
  text: string
  title?: string
  durationMs?: number
}

type ToastItem = ToastInput & {
  id: number
}

type ToastContextValue = {
  showToast: (toast: ToastInput) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const toneStyles: Record<ToastTone, string> = {
  success: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-200',
  error: 'border-rose-500/20 bg-rose-500/10 text-rose-200',
  info: 'border-blue-500/20 bg-blue-500/10 text-blue-200',
}

const toneIconStyles: Record<ToastTone, string> = {
  success: 'text-emerald-300',
  error: 'text-rose-300',
  info: 'text-blue-300',
}

const toneIcons = {
  success: CheckCircle2,
  error: AlertTriangle,
  info: Info,
} satisfies Record<ToastTone, typeof CheckCircle2>

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([])

  const dismissToast = useCallback((id: number) => {
    setToasts(current => current.filter(toast => toast.id !== id))
  }, [])

  const showToast = useCallback((toast: ToastInput) => {
    const id = Date.now() + Math.floor(Math.random() * 1000)
    const nextToast: ToastItem = {
      durationMs: 4200,
      ...toast,
      id,
    }
    setToasts(current => [...current, nextToast])
    window.setTimeout(() => dismissToast(id), nextToast.durationMs)
  }, [dismissToast])

  const value = useMemo<ToastContextValue>(() => ({ showToast }), [showToast])

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 top-4 z-[120] flex justify-center px-4">
        <div className="w-full max-w-md space-y-3">
          {toasts.map(toast => {
            const Icon = toneIcons[toast.tone]
            return (
              <div
                key={toast.id}
                role={toast.tone === 'error' ? 'alert' : 'status'}
                className={clsx(
                  'toast-card pointer-events-auto flex items-start gap-3 rounded-2xl border px-4 py-3 shadow-2xl',
                  toneStyles[toast.tone],
                )}
              >
                <Icon size={18} className={clsx('mt-0.5 flex-shrink-0', toneIconStyles[toast.tone])} />
                <div className="min-w-0 flex-1">
                  {toast.title && <div className="text-sm font-semibold text-white">{toast.title}</div>}
                  <div className="text-sm leading-5">{toast.text}</div>
                </div>
                <button
                  type="button"
                  className="rounded-lg p-1 text-slate-400 transition-colors hover:bg-slate-800/60 hover:text-white"
                  onClick={() => dismissToast(toast.id)}
                  aria-label="Dismiss notification"
                >
                  <X size={14} />
                </button>
              </div>
            )
          })}
        </div>
      </div>
    </ToastContext.Provider>
  )
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast must be used within ToastProvider')
  }
  return ctx
}
