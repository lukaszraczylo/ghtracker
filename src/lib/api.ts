import type { TrackerState } from './types'

export async function fetchState(signal?: AbortSignal): Promise<TrackerState> {
  const res = await fetch('/api/state', { signal, headers: { Accept: 'application/json' } })
  if (!res.ok) throw new Error(`state request failed: HTTP ${res.status}`)
  return (await res.json()) as TrackerState
}

/** Thrown when the server refuses a rescan because one ran recently. */
export class RescanLimitedError extends Error {}

export async function requestRefresh(): Promise<void> {
  const res = await fetch('/refresh', { method: 'POST' })
  if (res.status === 429) throw new RescanLimitedError('Rescan is limited to once every 5 minutes')
  if (!res.ok) throw new Error(`rescan request failed: HTTP ${res.status}`)
}
