export type Severity = 'ok' | 'warn' | 'crit'

export interface Release {
  tag: string
  url: string
  publishedAt: string
  prerelease: boolean
  fromTag: boolean
}

export interface Repo {
  fullName: string
  url: string
  releasesUrl: string
  description: string
  health: Severity
  error: string
  workflows: { total: number; failing: number; running: number }
  prs: { open: number; waiting: number; failingChecks: number; oldestSeconds: number }
  issues: { open: number; stale: number }
  archived: boolean
  up: boolean
  refreshedAt: string
  release: Release | null
}

export interface Alert {
  repo: string
  kind: string
  severity: Severity
  subject: string
  detail: string
  url: string
  since: string
}

export interface Thresholds {
  prWarnSeconds: number
  prCritSeconds: number
  issueStaleSeconds: number
  workflowStuckSeconds: number
  workflowDormantSeconds: number
}

export interface TrackerState {
  now: string
  lastRefresh: string
  intervalSeconds: number
  loaded: boolean
  refreshing: boolean
  manualRefresh: boolean
  scan: { done: number; total: number }
  counts: { crit: number; warn: number }
  thresholds: Thresholds
  repos: Repo[]
  alerts: Alert[]
}

/** One line in a repo block: every alert that targets the same URL, merged. */
export interface Row {
  kind: string
  subject: string
  detail: string
  url: string
  since: string
  severity: Exclude<Severity, 'ok'>
}

export interface Block {
  repo: Repo
  rows: Row[]
}
