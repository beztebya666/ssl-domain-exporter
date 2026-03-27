import type { ChangeEvent } from 'react'
import type { CustomField } from '../types'
import { enabledCustomFields, updateMetadataValue } from '../lib/customFields'

interface Props {
  fields: CustomField[]
  metadata: Record<string, string>
  onChange: (value: Record<string, string>) => void
}

export default function CustomFieldInputs({ fields, metadata, onChange }: Props) {
  const visibleFields = enabledCustomFields(fields)

  if (visibleFields.length === 0) {
    return null
  }

  return (
    <div className="space-y-4">
      <div className="text-sm font-medium text-white">Custom inventory fields</div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        {visibleFields.map(field => {
          const value = metadata[field.key] ?? ''
          return (
            <div key={field.key} className={field.type === 'textarea' ? 'md:col-span-2' : ''}>
              <label className="label" htmlFor={`custom-field-${field.key}`}>
                {field.label}
                {field.required && <span className="ml-1 text-rose-400">*</span>}
              </label>
              {renderFieldInput(field, value, nextValue => onChange(updateMetadataValue(metadata, field.key, nextValue)))}
              {field.help_text && <p className="mt-1 text-xs text-gray-500">{field.help_text}</p>}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function renderFieldInput(field: CustomField, value: string, onChange: (value: string) => void) {
  const common = {
    id: `custom-field-${field.key}`,
    className: 'input',
    value,
    required: field.required,
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => onChange(event.target.value),
  }

  switch (field.type) {
    case 'textarea':
      return (
        <textarea
          {...common}
          className="input h-28 resize-y"
          placeholder={field.placeholder || field.label}
        />
      )
    case 'email':
    case 'url':
    case 'date':
      return (
        <input
          {...common}
          type={field.type}
          placeholder={field.placeholder || field.label}
        />
      )
    case 'select':
      return (
        <select {...common} className="select">
          <option value="">Select {field.label}</option>
          {(field.options ?? []).map(option => (
            <option key={`${field.key}-${option.value}`} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      )
    case 'text':
    default:
      return (
        <input
          {...common}
          type="text"
          placeholder={field.placeholder || field.label}
        />
      )
  }
}
