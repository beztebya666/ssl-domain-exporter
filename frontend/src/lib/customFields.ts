import type { CustomField } from '../types'

export function normalizeCustomField(field: CustomField): CustomField {
  return {
    ...field,
    options: field.options ?? [],
  }
}

export function normalizeCustomFields(fields: CustomField[] | undefined): CustomField[] {
  return (fields ?? []).map(normalizeCustomField)
}

export function enabledCustomFields(fields: CustomField[] | undefined): CustomField[] {
  return normalizeCustomFields(fields).filter(field => field.enabled).sort((a, b) => a.sort_order - b.sort_order || a.label.localeCompare(b.label))
}

export function visibleCustomFields(fields: CustomField[] | undefined, visibility: 'table' | 'details' | 'export'): CustomField[] {
  return enabledCustomFields(fields).filter(field => {
    switch (visibility) {
      case 'table':
        return field.visible_in_table
      case 'details':
        return field.visible_in_details
      case 'export':
        return field.visible_in_export
      default:
        return false
    }
  })
}

export function filterableCustomFields(fields: CustomField[] | undefined): CustomField[] {
  return enabledCustomFields(fields).filter(field => field.filterable)
}

export function splitMetadataBySchema(metadata: Record<string, string> | undefined, fields: CustomField[] | undefined): {
  schemaMetadata: Record<string, string>
  extraMetadata: Record<string, string>
} {
  const schemaKeys = new Set(enabledCustomFields(fields).map(field => field.key))
  const schemaMetadata: Record<string, string> = {}
  const extraMetadata: Record<string, string> = {}

  Object.entries(metadata ?? {}).forEach(([key, value]) => {
    if (schemaKeys.has(key)) {
      schemaMetadata[key] = value
      return
    }
    extraMetadata[key] = value
  })

  return { schemaMetadata, extraMetadata }
}

export function mergeSchemaAndExtraMetadata(schemaMetadata: Record<string, string>, extraMetadata: Record<string, string>): Record<string, string> {
  return {
    ...extraMetadata,
    ...schemaMetadata,
  }
}

export function updateMetadataValue(metadata: Record<string, string>, key: string, value: string): Record<string, string> {
  const next = { ...metadata }
  const trimmed = value.trim()
  if (!trimmed) {
    delete next[key]
    return next
  }
  next[key] = trimmed
  return next
}

export function visibleMetadataSummary(metadata: Record<string, string> | undefined, fields: CustomField[] | undefined, visibility: 'table' | 'details'): Array<{ key: string; label: string; value: string }> {
  return visibleCustomFields(fields, visibility)
    .map(field => ({
      key: field.key,
      label: field.label,
      value: metadata?.[field.key] ?? '',
    }))
    .filter(item => item.value.trim() !== '')
}
