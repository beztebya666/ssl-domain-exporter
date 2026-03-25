import { useEffect } from 'react'
import type { ReactNode, MouseEvent } from 'react'
import clsx from 'clsx'
import { X } from 'lucide-react'

type Props = {
  title?: ReactNode
  description?: ReactNode
  children: ReactNode
  footer?: ReactNode
  onClose?: () => void
  panelClassName?: string
  bodyClassName?: string
  hideCloseButton?: boolean
}

export default function ModalShell({
  title,
  description,
  children,
  footer,
  onClose,
  panelClassName,
  bodyClassName,
  hideCloseButton = false,
}: Props) {
  useEffect(() => {
    if (!onClose) return undefined
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onClose()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  const handleOverlayClick = () => {
    onClose?.()
  }

  const stopPropagation = (event: MouseEvent<HTMLDivElement>) => {
    event.stopPropagation()
  }

  return (
    <div className="modal-overlay overflow-y-auto md:flex md:items-center md:justify-center" onClick={handleOverlayClick}>
      <div
        role="dialog"
        aria-modal="true"
        className={clsx('modal-panel', panelClassName)}
        onClick={stopPropagation}
      >
        {(title || description || (!hideCloseButton && onClose)) && (
          <div className="modal-header">
            <div className="min-w-0">
              {title && <div className="font-semibold text-white">{title}</div>}
              {description && <div className="mt-1 text-xs text-slate-500">{description}</div>}
            </div>
            {!hideCloseButton && onClose && (
              <button type="button" onClick={onClose} className="btn-ghost rounded-lg p-1.5" aria-label="Close dialog">
                <X size={16} />
              </button>
            )}
          </div>
        )}

        <div className={clsx('modal-body', bodyClassName)}>
          {children}
        </div>

        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  )
}
