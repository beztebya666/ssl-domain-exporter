import { useMemo, useState } from 'react'
import type { Dispatch, ReactNode, SetStateAction } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GripHorizontal, Pencil, Plus, Save, Settings2, Trash2, X } from 'lucide-react'
import { createCustomField, deleteCustomField, fetchCustomFields, updateCustomField } from '../api/client'
import type { CustomField, CustomFieldOption, CustomFieldWritePayload } from '../types'
import ConfirmDialog from './ConfirmDialog'
import EmptyState from './EmptyState'
import { Skeleton } from './Skeleton'
import { useToast } from './ToastProvider'
import { getErrorMessage } from '../lib/utils'

const FIELD_TYPES: Array<CustomField['type']> = ['text', 'textarea', 'email', 'url', 'date', 'select']

function emptyDraft(nextSortOrder: number): CustomFieldWritePayload {
  return {
    key: '',
    label: '',
    type: 'text',
    required: false,
    placeholder: '',
    help_text: '',
    sort_order: nextSortOrder,
    visible_in_table: false,
    visible_in_details: true,
    visible_in_export: false,
    filterable: false,
    enabled: true,
    options: [],
  }
}

export default function CustomFieldManager() {
  const qc = useQueryClient()
  const { showToast } = useToast()
  const { data: fields = [], isLoading } = useQuery({
    queryKey: ['custom-fields', 'admin'],
    queryFn: () => fetchCustomFields(true),
  })

  const [editingID, setEditingID] = useState<number | 'new' | null>(null)
  const [draft, setDraft] = useState<CustomFieldWritePayload | null>(null)
  const [pendingDelete, setPendingDelete] = useState<CustomField | null>(null)

  const nextSortOrder = useMemo(
    () => Math.max(0, ...fields.map(field => field.sort_order || 0)) + 1,
    [fields],
  )

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['custom-fields'] })
    qc.invalidateQueries({ queryKey: ['domains-page'] })
    qc.invalidateQueries({ queryKey: ['dashboard-domains-page'] })
    qc.invalidateQueries({ queryKey: ['domain'] })
  }

  const createMutation = useMutation({
    mutationFn: createCustomField,
    onSuccess: (field) => {
      invalidate()
      setEditingID(null)
      setDraft(null)
      showToast({ tone: 'success', text: `Custom field "${field.label}" created.` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to create custom field.') }),
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, payload }: { id: number; payload: CustomFieldWritePayload }) => updateCustomField(id, payload),
    onSuccess: (field) => {
      invalidate()
      setEditingID(null)
      setDraft(null)
      showToast({ tone: 'success', text: `Custom field "${field.label}" updated.` })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to update custom field.') }),
  })

  const deleteMutation = useMutation({
    mutationFn: deleteCustomField,
    onSuccess: () => {
      invalidate()
      setEditingID(null)
      setDraft(null)
      showToast({ tone: 'success', text: 'Custom field deleted. Existing values remain as extra metadata.' })
    },
    onError: (err: unknown) => showToast({ tone: 'error', text: getErrorMessage(err, 'Failed to delete custom field.') }),
  })

  const beginCreate = () => {
    setEditingID('new')
    setDraft(emptyDraft(nextSortOrder))
  }

  const beginEdit = (field: CustomField) => {
    setEditingID(field.id)
    setDraft({
      key: field.key,
      label: field.label,
      type: field.type,
      required: field.required,
      placeholder: field.placeholder,
      help_text: field.help_text,
      sort_order: field.sort_order,
      visible_in_table: field.visible_in_table,
      visible_in_details: field.visible_in_details,
      visible_in_export: field.visible_in_export,
      filterable: field.filterable,
      enabled: field.enabled,
      options: field.options.map(option => ({
        value: option.value,
        label: option.label,
        sort_order: option.sort_order,
      })),
    })
  }

  const cancelEdit = () => {
    setEditingID(null)
    setDraft(null)
  }

  const save = () => {
    if (!draft) return
    if (editingID === 'new') {
      createMutation.mutate(normalizeDraft(draft))
      return
    }
    if (typeof editingID === 'number') {
      updateMutation.mutate({ id: editingID, payload: normalizeDraft(draft) })
    }
  }

  const remove = (field: CustomField) => {
    setPendingDelete(field)
  }

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 text-xs text-slate-400">
        Define optional inventory fields once and render them consistently across add/edit forms, filters, details, and export.
        Metadata values continue to live in the existing domain metadata JSON, so this layer stays backwards-compatible.
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-sm font-semibold text-white">Configured fields</div>
        <button type="button" className="btn-primary" onClick={beginCreate}>
          <Plus size={14} />
          Add custom field
        </button>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, index) => (
            <div key={index} className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
              <Skeleton className="h-5 w-52" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-3/4" />
            </div>
          ))}
        </div>
      ) : fields.length === 0 ? (
        <EmptyState
          icon={Settings2}
          title="No custom fields configured yet"
          description="Define optional inventory fields for owners, zones, change IDs, or other enterprise metadata and reuse them across forms, filters, details, and export."
        />
      ) : (
        <div className="space-y-3">
          {fields.map(field => (
            <div key={field.id} className="rounded-xl border border-slate-800 bg-slate-900/40 p-4">
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <GripHorizontal size={14} className="text-slate-600" />
                    <div className="text-sm font-semibold text-white">{field.label}</div>
                    <span className="rounded-full border border-slate-700 bg-slate-950/40 px-2 py-0.5 text-[11px] text-slate-300">{field.type}</span>
                    {field.required && <FlagBadge tone="amber" text="required" />}
                    {field.filterable && <FlagBadge tone="blue" text="filter" />}
                    {field.visible_in_table && <FlagBadge tone="emerald" text="table" />}
                    {field.visible_in_details && <FlagBadge tone="violet" text="details" />}
                    {field.visible_in_export && <FlagBadge tone="cyan" text="export" />}
                    {!field.enabled && <FlagBadge tone="slate" text="disabled" />}
                  </div>
                  <div className="mt-1 text-xs text-slate-400">
                    Key: <span className="font-mono text-slate-300">{field.key}</span>
                    {field.placeholder && <span> | Placeholder: {field.placeholder}</span>}
                    {field.help_text && <span> | {field.help_text}</span>}
                  </div>
                  {field.options.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-2">
                      {field.options.map(option => (
                        <span key={`${field.key}-${option.value}`} className="rounded-full border border-slate-700 bg-slate-950/50 px-2 py-0.5 text-[11px] text-slate-300">
                          {option.label}
                        </span>
                      ))}
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  <button type="button" className="btn-ghost border border-slate-700" onClick={() => beginEdit(field)}>
                    <Pencil size={14} />
                    Edit
                  </button>
                  <button type="button" className="btn-danger" onClick={() => remove(field)} disabled={deleteMutation.isPending}>
                    <Trash2 size={14} />
                    Delete
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {draft && (
        <div className="rounded-2xl border border-blue-500/20 bg-blue-500/5 p-5 space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="text-sm font-semibold text-white">{editingID === 'new' ? 'Create custom field' : 'Edit custom field'}</div>
              <div className="mt-1 text-xs text-slate-400">
                Field keys are immutable after creation so existing metadata values stay stable and safe.
              </div>
            </div>
            <div className="flex items-center gap-2">
              <button type="button" className="btn-ghost border border-slate-700" onClick={cancelEdit}>
                <X size={14} />
                Cancel
              </button>
              <button
                type="button"
                className="btn-primary"
                onClick={save}
                disabled={createMutation.isPending || updateMutation.isPending}
              >
                <Save size={14} />
                {createMutation.isPending || updateMutation.isPending ? 'Saving...' : 'Save field'}
              </button>
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <Field label="Key" htmlFor="custom-field-key" hint="Lowercase identifier used in metadata JSON and import/export keys">
              <input
                id="custom-field-key"
                className="input font-mono"
                value={draft.key}
                onChange={e => setDraft(current => current ? { ...current, key: e.target.value } : current)}
                disabled={editingID !== 'new'}
                placeholder="owner_email"
              />
            </Field>
            <Field label="Label" htmlFor="custom-field-label" hint="Human-friendly field name shown in the UI">
              <input
                id="custom-field-label"
                className="input"
                value={draft.label}
                onChange={e => setDraft(current => current ? { ...current, label: e.target.value } : current)}
                placeholder="Owner Email"
              />
            </Field>
            <Field label="Type" htmlFor="custom-field-type">
              <select
                id="custom-field-type"
                className="select"
                value={draft.type}
                onChange={e => setDraft(current => current ? {
                  ...current,
                  type: e.target.value as CustomField['type'],
                  options: e.target.value === 'select' ? (current.options.length > 0 ? current.options : [{ value: '', label: '' }]) : [],
                } : current)}
              >
                {FIELD_TYPES.map(type => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </Field>
            <Field label="Placeholder" htmlFor="custom-field-placeholder">
              <input
                id="custom-field-placeholder"
                className="input"
                value={draft.placeholder}
                onChange={e => setDraft(current => current ? { ...current, placeholder: e.target.value } : current)}
                placeholder="team@example.com"
              />
            </Field>
            <Field label="Help text" htmlFor="custom-field-help-text">
              <input
                id="custom-field-help-text"
                className="input"
                value={draft.help_text}
                onChange={e => setDraft(current => current ? { ...current, help_text: e.target.value } : current)}
                placeholder="Shown below the input to guide operators"
              />
            </Field>
            <Field label="Sort order" htmlFor="custom-field-sort-order">
              <input
                id="custom-field-sort-order"
                className="input"
                type="number"
                min={1}
                value={draft.sort_order}
                onChange={e => setDraft(current => current ? { ...current, sort_order: Number(e.target.value) || 1 } : current)}
              />
            </Field>
          </div>

          <div className="grid grid-cols-1 gap-3 md:grid-cols-3 xl:grid-cols-6">
            <ToggleCard label="Required" hint="Enforce a value on create, update, and import." checked={draft.required} onChange={checked => setDraft(current => current ? { ...current, required: checked } : current)} />
            <ToggleCard label="Enabled" hint="Disable to hide the field while preserving stored values." checked={draft.enabled} onChange={checked => setDraft(current => current ? { ...current, enabled: checked } : current)} />
            <ToggleCard label="Filterable" hint="Show as a first-class filter on list views." checked={draft.filterable} onChange={checked => setDraft(current => current ? { ...current, filterable: checked } : current)} />
            <ToggleCard label="Visible in table" hint="Display summarized value in inventory list cards." checked={draft.visible_in_table} onChange={checked => setDraft(current => current ? { ...current, visible_in_table: checked } : current)} />
            <ToggleCard label="Visible in details" hint="Show in the detail page metadata card." checked={draft.visible_in_details} onChange={checked => setDraft(current => current ? { ...current, visible_in_details: checked } : current)} />
            <ToggleCard label="Visible in export" hint="Append this field as its own CSV export column." checked={draft.visible_in_export} onChange={checked => setDraft(current => current ? { ...current, visible_in_export: checked } : current)} />
          </div>

          {draft.type === 'select' && (
            <div className="rounded-xl border border-slate-800 bg-slate-950/40 p-4 space-y-3">
              <div className="flex items-center justify-between gap-3">
                <div className="text-sm font-semibold text-white">Select options</div>
                <button
                  type="button"
                  className="btn-ghost border border-slate-700"
                  onClick={() => setDraft(current => current ? {
                    ...current,
                    options: [...current.options, { value: '', label: '' }],
                  } : current)}
                >
                  <Plus size={14} />
                  Add option
                </button>
              </div>
              <div className="space-y-3">
                {draft.options.map((option, index) => (
                  <div key={`option-${index}`} className="grid grid-cols-1 gap-3 md:grid-cols-[1fr_1fr_auto]">
                    <Field label={`Value ${index + 1}`} htmlFor={`custom-field-option-value-${index}`}>
                      <input
                        id={`custom-field-option-value-${index}`}
                        className="input"
                        value={option.value}
                        onChange={e => updateOption(setDraft, index, { ...option, value: e.target.value })}
                        placeholder="production"
                      />
                    </Field>
                    <Field label={`Label ${index + 1}`} htmlFor={`custom-field-option-label-${index}`}>
                      <input
                        id={`custom-field-option-label-${index}`}
                        className="input"
                        value={option.label}
                        onChange={e => updateOption(setDraft, index, { ...option, label: e.target.value })}
                        placeholder="Production"
                      />
                    </Field>
                    <div className="flex items-end">
                      <button
                        type="button"
                        className="btn-danger"
                        onClick={() => setDraft(current => current ? {
                          ...current,
                          options: current.options.filter((_, optionIndex) => optionIndex !== index),
                        } : current)}
                      >
                        <Trash2 size={14} />
                        Remove
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title="Delete custom field"
        description={pendingDelete ? `Delete "${pendingDelete.label}"? Existing values will remain in metadata as extra fields, but this schema definition will be removed.` : ''}
        confirmLabel="Delete field"
        busy={deleteMutation.isPending}
        onClose={() => setPendingDelete(null)}
        onConfirm={() => {
          if (!pendingDelete) return
          deleteMutation.mutate(pendingDelete.id, {
            onSettled: () => setPendingDelete(null),
          })
        }}
      />
    </div>
  )
}

function normalizeDraft(draft: CustomFieldWritePayload): CustomFieldWritePayload {
  return {
    ...draft,
    key: draft.key.trim().toLowerCase(),
    label: draft.label.trim(),
    placeholder: draft.placeholder.trim(),
    help_text: draft.help_text.trim(),
    sort_order: draft.sort_order > 0 ? draft.sort_order : 1,
    options: draft.type === 'select'
      ? draft.options
        .map((option, index) => ({
          value: option.value.trim(),
          label: option.label.trim(),
          sort_order: index + 1,
        }))
        .filter(option => option.value !== '')
      : [],
  }
}

function updateOption(
  setDraft: Dispatch<SetStateAction<CustomFieldWritePayload | null>>,
  index: number,
  nextOption: CustomFieldOption,
) {
  setDraft(current => {
    if (!current) return current
    const nextOptions = [...current.options]
    nextOptions[index] = nextOption
    return {
      ...current,
      options: nextOptions,
    }
  })
}

function Field({ label, htmlFor, hint, children }: { label: string; htmlFor: string; hint?: string; children: ReactNode }) {
  return (
    <div>
      <label className="label" htmlFor={htmlFor}>{label}</label>
      {children}
      {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  )
}

function ToggleCard({ label, hint, checked, onChange }: { label: string; hint: string; checked: boolean; onChange: (checked: boolean) => void }) {
  return (
    <label className="rounded-xl border border-slate-800 bg-slate-950/40 p-3 text-sm text-slate-300">
      <div className="flex items-center justify-between gap-3">
        <span className="font-medium text-white">{label}</span>
        <input className="checkbox" type="checkbox" checked={checked} onChange={e => onChange(e.target.checked)} />
      </div>
      <div className="mt-2 text-xs text-slate-500">{hint}</div>
    </label>
  )
}

function FlagBadge({ tone, text }: { tone: 'amber' | 'blue' | 'emerald' | 'violet' | 'cyan' | 'slate'; text: string }) {
  const toneClass = {
    amber: 'border-amber-500/30 bg-amber-500/10 text-amber-300',
    blue: 'border-blue-500/30 bg-blue-500/10 text-blue-300',
    emerald: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300',
    violet: 'border-violet-500/30 bg-violet-500/10 text-violet-300',
    cyan: 'border-cyan-500/30 bg-cyan-500/10 text-cyan-300',
    slate: 'border-slate-700 bg-slate-800/70 text-slate-300',
  }[tone]

  return <span className={`rounded-full border px-2 py-0.5 text-[11px] ${toneClass}`}>{text}</span>
}
