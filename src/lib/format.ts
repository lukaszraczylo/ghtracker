const HOUR = 3600
const DAY = 24 * HOUR

/** Compact age such as "3d", "5h" or "12m"; mirrors the Go collector.Age helper. */
export function ageLabel(seconds: number): string {
  if (seconds < 0) return '0m'
  if (seconds >= 2 * DAY) return `${Math.floor(seconds / DAY)}d`
  if (seconds >= HOUR) return `${Math.floor(seconds / HOUR)}h`
  return `${Math.floor(seconds / 60)}m`
}

/** Seconds between an RFC 3339 timestamp and nowMs. */
export function secondsSince(iso: string, nowMs: number): number {
  return Math.floor((nowMs - Date.parse(iso)) / 1000)
}

export function ageSince(iso: string, nowMs: number): string {
  return Number.isNaN(Date.parse(iso)) ? '' : ageLabel(secondsSince(iso, nowMs))
}

/** Go encodes the zero time as year 1; treat it as "unknown". */
export function isZeroTime(iso: string): boolean {
  return !iso || iso.startsWith('0001-')
}

export function shorten(text: string, max = 90): string {
  const chars = Array.from(text)
  return chars.length <= max ? text : chars.slice(0, max - 1).join('') + '…'
}
