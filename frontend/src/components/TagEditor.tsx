import { useState } from 'react'
import { X } from 'lucide-react'
import { normalizeTags, parseTagText } from '../lib/domainFields'

interface Props {
  tags: string[]
  onChange: (tags: string[]) => void
  placeholder?: string
}

export default function TagEditor({ tags, onChange, placeholder = 'Add tag and press Enter' }: Props) {
  const [draft, setDraft] = useState('')

  const commit = (raw: string) => {
    const nextTags = parseTagText(raw)
    if (nextTags.length === 0) return
    onChange(normalizeTags([...tags, ...nextTags]))
    setDraft('')
  }

  const removeTag = (tag: string) => {
    onChange(tags.filter(item => item.toLowerCase() !== tag.toLowerCase()))
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {tags.length === 0 && (
          <span className="text-xs text-gray-600">No tags yet</span>
        )}
        {tags.map(tag => (
          <span key={tag.toLowerCase()} className="inline-flex items-center gap-1 rounded-full border border-blue-500/20 bg-blue-500/10 px-2.5 py-1 text-xs text-blue-300">
            {tag}
            <button type="button" className="opacity-70 hover:opacity-100" onClick={() => removeTag(tag)} aria-label={`Remove tag ${tag}`}>
              <X size={12} />
            </button>
          </span>
        ))}
      </div>

      <input
        className="input"
        placeholder={placeholder}
        value={draft}
        onChange={e => setDraft(e.target.value)}
        onKeyDown={e => {
          if (e.key === 'Enter' || e.key === ',' || e.key === 'Tab') {
            e.preventDefault()
            commit(draft)
            return
          }
          if (e.key === 'Backspace' && draft.length === 0 && tags.length > 0) {
            e.preventDefault()
            removeTag(tags[tags.length - 1])
          }
        }}
        onBlur={() => commit(draft)}
        onPaste={e => {
          const text = e.clipboardData.getData('text')
          if (!/[\n,;\t ]/.test(text)) return
          e.preventDefault()
          commit(text)
        }}
      />

      <p className="text-xs text-gray-500">Use Enter, comma, Tab, or paste multiple values to add tags. Click a tag to remove it.</p>
    </div>
  )
}
