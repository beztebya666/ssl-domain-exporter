export function normalizeTags(tags: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []

  for (const raw of tags) {
    const tag = raw.trim()
    if (!tag) continue
    const key = tag.toLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    out.push(tag)
  }

  return out
}

export function parseTagText(raw: string): string[] {
  if (!raw.trim()) return []
  return normalizeTags(raw.split(/[\s,;\n\r\t]+/g))
}

export function tagsToText(tags: string[]): string {
  return normalizeTags(tags).join(', ')
}

export function metadataSearchText(metadata: Record<string, string> | undefined): string {
  if (!metadata) return ''
  return Object.entries(metadata)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join('; ')
}

export function normalizeMetadata(metadata: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {}
  const keys = Object.keys(metadata).sort((a, b) => a.localeCompare(b))

  for (const rawKey of keys) {
    const key = rawKey.trim().toLowerCase()
    const value = (metadata[rawKey] ?? '').trim()
    if (!key || !value) continue
    out[key] = value
  }

  return out
}
