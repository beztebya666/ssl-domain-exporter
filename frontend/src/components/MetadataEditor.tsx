import { useRef, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { normalizeMetadata } from '../lib/domainFields'

interface MetadataRow {
  id: string
  key: string
  value: string
}

interface Props {
  value: Record<string, string>
  onChange: (value: Record<string, string>) => void
}

function rowsFromMetadata(value: Record<string, string>): MetadataRow[] {
  return Object.entries(value)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, entryValue], index) => ({
      id: `${key}-${index}`,
      key,
      value: entryValue,
    }))
}

function metadataFromRows(rows: MetadataRow[]): Record<string, string> {
  const out: Record<string, string> = {}
  for (const row of rows) {
    if (!row.key.trim() || !row.value.trim()) continue
    out[row.key] = row.value
  }
  return normalizeMetadata(out)
}

export default function MetadataEditor({ value, onChange }: Props) {
  const [rows, setRows] = useState<MetadataRow[]>(() => rowsFromMetadata(value))
  const nextID = useRef(0)

  const updateRows = (nextRows: MetadataRow[]) => {
    setRows(nextRows)
    onChange(metadataFromRows(nextRows))
  }

  const addRow = () => {
    const id = `new-${nextID.current++}`
    updateRows([...rows, { id, key: '', value: '' }])
  }

  return (
    <div className="space-y-2">
      {rows.length === 0 && (
        <div className="text-xs text-gray-600">No metadata yet</div>
      )}

      <div className="space-y-2">
        {rows.map((row, index) => (
          <div key={row.id} className="grid grid-cols-[1fr_1fr_auto] gap-2">
            <input
              className="input"
              placeholder="owner"
              value={row.key}
              onChange={e => {
                const nextRows = [...rows]
                nextRows[index] = { ...row, key: e.target.value }
                updateRows(nextRows)
              }}
            />
            <input
              className="input"
              placeholder="Platform Team"
              value={row.value}
              onChange={e => {
                const nextRows = [...rows]
                nextRows[index] = { ...row, value: e.target.value }
                updateRows(nextRows)
              }}
            />
            <button
              type="button"
              className="btn-ghost border border-gray-700 px-3"
              onClick={() => updateRows(rows.filter(item => item.id !== row.id))}
              aria-label={`Remove metadata row ${row.key || index + 1}`}
            >
              <Trash2 size={14} />
            </button>
          </div>
        ))}
      </div>

      <button type="button" className="btn-ghost border border-gray-700 text-xs" onClick={addRow}>
        <Plus size={13} />
        Add metadata field
      </button>

      <p className="text-xs text-gray-500">Use metadata for owner, zone, requester, change ID, environment, or other inventory fields.</p>
    </div>
  )
}
