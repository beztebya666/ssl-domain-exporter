export const REDACTED_SECRET = '__REDACTED__'

export function isRedactedSecret(value: string | null | undefined): boolean {
  return value === REDACTED_SECRET
}

export function secretInputValue(value: string | null | undefined): string {
  return isRedactedSecret(value) ? '' : (value ?? '')
}

export function secretPlaceholder(value: string | null | undefined): string | undefined {
  return isRedactedSecret(value) ? 'Configured. Leave blank to keep the current value.' : undefined
}
