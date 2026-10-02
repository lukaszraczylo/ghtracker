import type { RepoRefreshResult, TrackerState } from './types'

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

/** Thrown when one repository was refreshed too recently or a scan is running. */
export class RepoRefreshRefusedError extends Error {}

const REPO_COOLDOWN_FALLBACK_SECONDS = 60

/** Refreshes one repository and returns the full state after it, plus the repo name. */
export async function requestRepoRefresh(fullName: string): Promise<RepoRefreshResult> {
  const res = await fetch(`/refresh?repo=${encodeURIComponent(fullName)}`, { method: 'POST' })
  if (res.status === 429) {
    const wait = Number(res.headers.get('Retry-After')) || REPO_COOLDOWN_FALLBACK_SECONDS
    throw new RepoRefreshRefusedError(`${fullName} was refreshed recently. Try again in ${wait}s`)
  }
  if (res.status === 409)
    throw new RepoRefreshRefusedError('A refresh is already running. Try again shortly')
  if (!res.ok) throw new Error(`refresh of ${fullName} failed: HTTP ${res.status}`)
  return (await res.json()) as RepoRefreshResult
}
