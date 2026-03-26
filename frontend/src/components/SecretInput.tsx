import { isRedactedSecret, secretInputValue, secretPlaceholder } from '../lib/secrets'

type SecretInputProps = {
  value: string
  onChange: (value: string) => void
  type?: 'text' | 'password'
  ariaLabel?: string
}

export default function SecretInput({
  value,
  onChange,
  type = 'password',
  ariaLabel = 'Secret value',
}: SecretInputProps) {
  const configured = isRedactedSecret(value)
  const hasValue = configured || value.trim() !== ''

  return (
    <div className="space-y-2">
      <div className="flex gap-2">
        <input
          className="input flex-1"
          type={type}
          value={secretInputValue(value)}
          placeholder={secretPlaceholder(value)}
          aria-label={ariaLabel}
          onChange={e => onChange(e.target.value)}
        />
        <button
          type="button"
          className="btn-ghost shrink-0 border border-slate-700"
          disabled={!hasValue}
          onClick={() => onChange('')}
        >
          Clear
        </button>
      </div>
      {configured && (
        <p className="text-xs text-slate-500">
          A saved value exists. Leave the field untouched to keep it, or click Clear to remove it.
        </p>
      )}
    </div>
  )
}
