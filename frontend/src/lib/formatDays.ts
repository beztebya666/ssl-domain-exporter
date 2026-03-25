export function formatDays(days: number): string {
  if (days < 0) return `Expired ${Math.abs(days)}d ago`
  if (days === 0) return 'Expires today'
  return `${days}d`
}
