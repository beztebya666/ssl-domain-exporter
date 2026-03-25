import type { KeyboardEvent } from 'react'

export function activateCardOnKey(event: KeyboardEvent, onActivate: () => void) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    onActivate()
  }
}

export function parseOptionalInt(value: string): number | undefined {
  const trimmed = value.trim()
  if (!trimmed) return undefined
  const parsed = Number(trimmed)
  if (!Number.isFinite(parsed) || parsed < 0) return undefined
  return Math.trunc(parsed)
}

export function updateFilter(current: Record<string, string>, key: string, value: string): Record<string, string> {
  const next = { ...current }
  const trimmed = value.trim()
  if (!trimmed) {
    delete next[key]
    return next
  }
  next[key] = trimmed
  return next
}

export function getErrorMessage(err: unknown, fallback: string): string {
  const maybeResponse = (err as { response?: { data?: { error?: string } } } | undefined)?.response
  if (maybeResponse?.data?.error) return maybeResponse.data.error
  if (err instanceof Error && err.message) return err.message
  return fallback
}

export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName.toLowerCase()
  return tag === 'input' || tag === 'textarea' || tag === 'select' || target.isContentEditable
}
