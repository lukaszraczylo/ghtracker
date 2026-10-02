import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { fetchState, RescanLimitedError, requestRefresh } from '@/lib/api'
import {
  activeFilterCount,
  applyFilters,
  emptyFilters,
  facetCounts,
  ownerOf,
  repoMatches,
  type Filters,
} from '@/lib/filters'
import { DATA_KINDS } from '@/lib/kinds'
import type { Block, Row, TrackerState } from '@/lib/types'

/** While a rescan runs the server takes ~40s, so check back often instead of every minute. */
export const SCAN_POLL_MS = 2000
const SCAN_TIMEOUT_MS = 10 * 60 * 1000

const SEVERITY_RANK = { ok: 0, warn: 1, crit: 2 } as const

/** Groups alerts into one row per target URL and one block per unhealthy repo. */
export function buildBlocks(state: TrackerState): Block[] {
  const rows = new Map<string, Row>()
  const order = new Map<string, string[]>()
  for (const alert of state.alerts) {
    if (alert.severity === 'ok') continue
    const key = `${alert.repo}\n${alert.url}`
    const existing = rows.get(key)
    if (existing) {
      existing.detail += ` · ${alert.detail}`
      if (SEVERITY_RANK[alert.severity] > SEVERITY_RANK[existing.severity]) {
        existing.severity = alert.severity
      }
      continue
    }
    rows.set(key, {
      kind: alert.kind,
      subject: alert.subject,
      detail: alert.detail,
      url: alert.url,
      since: alert.since,
      severity: alert.severity,
    })
    order.set(alert.repo, [...(order.get(alert.repo) ?? []), key])
  }
  const blocks: Block[] = []
  for (const repo of state.repos) {
    const keys = order.get(repo.fullName)
    if (!keys) continue
    const list = keys.map((k) => rows.get(k)!)
    list.sort((a, b) => SEVERITY_RANK[b.severity] - SEVERITY_RANK[a.severity])
    blocks.push({ repo, rows: list })
  }
  return blocks
}

export interface RepoCounts {
  crit: number
  warn: number
}

export interface Facts {
  failingWorkflows: number
  waitingPrs: number
  failingChecks: number
  staleIssues: number
  dataProblems: number
}

/** Counts the alerts of each severity per repo, for the fleet strip. */
export function countByRepo(state: TrackerState): Map<string, RepoCounts> {
  const out = new Map<string, RepoCounts>()
  for (const a of state.alerts) {
    if (a.severity === 'ok') continue
    const c = out.get(a.repo) ?? { crit: 0, warn: 0 }
    c[a.severity] += 1
    out.set(a.repo, c)
  }
  return out
}

/** Headline numbers for the overview; issue counts come from the repos, not alert lines. */
export function computeFacts(state: TrackerState): Facts {
  const count = (kinds: string[]) => state.alerts.filter((a) => kinds.includes(a.kind)).length
  return {
    failingWorkflows: state.repos.reduce((n, r) => n + r.workflows.failing, 0),
    waitingPrs: count(['pr_waiting']),
    failingChecks: state.repos.reduce((n, r) => n + r.prs.failingChecks, 0),
    staleIssues: state.repos.reduce((n, r) => n + r.issues.stale, 0),
    dataProblems: count(DATA_KINDS),
  }
}

export const useTrackerStore = defineStore('tracker', () => {
  const state = ref<TrackerState | null>(null)
  const error = ref('')
  const loading = ref(false)
  const requesting = ref(false)
  const notice = ref('')
  const skewMs = ref(0)
  const tick = ref(Date.now())

  /** Server time advanced by the local clock, so ages stay right when the clocks differ. */
  const nowMs = computed(() => tick.value + skewMs.value)
  /** True from the click until the server finished the new scan. */
  const scanning = computed(() => requesting.value || (state.value?.refreshing ?? false))
  const blocks = computed(() => (state.value ? buildBlocks(state.value) : []))
  const filters = ref<Filters>(emptyFilters())
  const visibleBlocks = computed(() => applyFilters(blocks.value, filters.value))
  const owners = computed(() => [
    ...new Set((state.value?.repos ?? []).map((r) => ownerOf(r.fullName))),
  ])
  const facets = computed(() => facetCounts(blocks.value, filters.value, owners.value))
  /** Repos that match by name or version, plus repos owning an item the search matched. */
  const visibleRepos = computed(() => {
    const withItems = new Set(visibleBlocks.value.map((b) => b.repo.fullName))
    return (state.value?.repos ?? []).filter(
      (r) =>
        repoMatches(r, filters.value) ||
        (filters.value.query.trim() !== '' && withItems.has(r.fullName)),
    )
  })
  const activeFilters = computed(() => activeFilterCount(filters.value))
  /** Rows by severity, so the headline matches the lists below it (alerts on one item merge into one row). */
  const rowCounts = computed(() => {
    const out = { crit: 0, warn: 0 }
    for (const b of blocks.value) for (const r of b.rows) out[r.severity] += 1
    return out
  })
  const repoCounts = computed(() => (state.value ? countByRepo(state.value) : new Map()))
  const facts = computed(() => (state.value ? computeFacts(state.value) : null))

  function toggleRepo(fullName: string): void {
    filters.value.repo = filters.value.repo === fullName ? null : fullName
  }

  function clearFilters(): void {
    filters.value = emptyFilters()
  }

  function advanceClock(): void {
    tick.value = Date.now()
  }

  async function load(): Promise<void> {
    loading.value = true
    try {
      const next = await fetchState()
      skewMs.value = Date.parse(next.now) - Date.now()
      tick.value = Date.now()
      state.value = next
      error.value = ''
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e)
    } finally {
      loading.value = false
    }
  }

  /** Asks the server to rescan every repository, then follows it until the new data lands. */
  async function rescan(): Promise<void> {
    const before = state.value?.lastRefresh ?? ''
    requesting.value = true
    notice.value = ''
    try {
      await requestRefresh()
      error.value = ''
      const deadline = Date.now() + SCAN_TIMEOUT_MS
      do {
        await load()
        if (state.value && state.value.lastRefresh !== before && !state.value.refreshing) break
        await new Promise((resolve) => setTimeout(resolve, SCAN_POLL_MS))
      } while (Date.now() < deadline)
    } catch (e) {
      if (e instanceof RescanLimitedError) notice.value = e.message
      else error.value = e instanceof Error ? e.message : String(e)
    } finally {
      requesting.value = false
    }
  }

  return {
    state,
    error,
    loading,
    notice,
    scanning,
    nowMs,
    blocks,
    filters,
    owners,
    facets,
    visibleRepos,
    activeFilters,
    visibleBlocks,
    repoCounts,
    rowCounts,
    facts,
    toggleRepo,
    clearFilters,
    load,
    rescan,
    advanceClock,
  }
})
