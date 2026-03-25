import { AlertTriangle } from 'lucide-react'
import ModalShell from './ModalShell'

type Props = {
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  cancelLabel?: string
  tone?: 'danger' | 'neutral'
  busy?: boolean
  onConfirm: () => void
  onClose: () => void
}

export default function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  tone = 'danger',
  busy = false,
  onConfirm,
  onClose,
}: Props) {
  if (!open) return null

  return (
    <ModalShell
      onClose={busy ? undefined : onClose}
      panelClassName="max-w-md"
      title={
        <div className="flex items-center gap-2">
          <AlertTriangle size={16} className={tone === 'danger' ? 'text-amber-400' : 'text-blue-400'} />
          {title}
        </div>
      }
      footer={
        <>
          <button type="button" className="btn-ghost flex-1 border border-slate-700" onClick={onClose} disabled={busy}>
            {cancelLabel}
          </button>
          <button
            type="button"
            className={tone === 'danger' ? 'btn-danger flex-1 justify-center' : 'btn-primary flex-1 justify-center'}
            onClick={onConfirm}
            disabled={busy}
          >
            {busy ? 'Working...' : confirmLabel}
          </button>
        </>
      }
    >
      <div className="text-sm text-slate-300">{description}</div>
    </ModalShell>
  )
}
