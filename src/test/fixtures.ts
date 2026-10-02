import type { Alert, Repo, TrackerState } from '@/lib/types'

export const NOW = '2026-10-02T12:00:00Z'
export const hoursAgo = (h: number) => new Date(Date.parse(NOW) - h * 3600_000).toISOString()

export function repo(over: Partial<Repo> = {}): Repo {
  return {
    fullName: 'o/r',
    url: 'https://github.com/o/r',
    releasesUrl: 'https://github.com/o/r/releases',
    description: '',
    health: 'ok',
    error: '',
    workflows: { total: 0, failing: 0, running: 0 },
    prs: { open: 0, waiting: 0, failingChecks: 0, oldestSeconds: 0 },
    issues: { open: 0, stale: 0 },
    archived: false,
    up: true,
    refreshedAt: hoursAgo(1),
    release: {
      tag: 'v1.0.0',
      url: 'https://github.com/o/r/releases/tag/v1.0.0',
      publishedAt: hoursAgo(72),
      prerelease: false,
      fromTag: false,
    },
    ...over,
  }
}

export function alert(over: Partial<Alert> = {}): Alert {
  return {
    repo: 'o/r',
    kind: 'workflow_failed',
    severity: 'crit',
    subject: 'CI',
    detail: 'failure on main (push)',
    url: 'https://github.com/o/r/actions/runs/1',
    since: hoursAgo(2),
    ...over,
  }
}

export function state(over: Partial<TrackerState> = {}): TrackerState {
  return {
    now: NOW,
    lastRefresh: hoursAgo(1),
    intervalSeconds: 21600,
    loaded: true,
    refreshing: false,
    manualRefresh: false,
    scan: { done: 0, total: 0 },
    counts: { crit: 0, warn: 0 },
    thresholds: {
      prWarnSeconds: 172800,
      prCritSeconds: 604800,
      issueStaleSeconds: 2592000,
      workflowStuckSeconds: 21600,
      workflowDormantSeconds: 2592000,
    },
    repos: [],
    alerts: [],
    ...over,
  }
}

/** A state with one failing repo (two alerts on one PR) and one healthy repo. */
export function mixedState(): TrackerState {
  return state({
    counts: { crit: 2, warn: 1 },
    repos: [
      repo({
        health: 'crit',
        workflows: { total: 3, failing: 1, running: 0 },
        prs: { open: 2, waiting: 1, failingChecks: 1, oldestSeconds: 777600 },
        issues: { open: 4, stale: 2 },
      }),
      repo({
        fullName: 'o/fine',
        url: 'https://github.com/o/fine',
        release: null,
        releasesUrl: 'https://github.com/o/fine/releases',
      }),
    ],
    alerts: [
      alert(),
      alert({
        kind: 'pr_waiting',
        severity: 'crit',
        subject: '#5 <b>bold</b> title',
        detail: 'open 9d',
        url: 'https://github.com/o/r/pull/5',
        since: hoursAgo(216),
      }),
      alert({
        kind: 'pr_checks_failing',
        severity: 'warn',
        subject: '#5 <b>bold</b> title',
        detail: 'checks failing: build',
        url: 'https://github.com/o/r/pull/5',
        since: hoursAgo(10),
      }),
    ],
  })
}
