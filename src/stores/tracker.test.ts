import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { alert, mixedState, repo, state } from '@/test/fixtures'
import { buildBlocks, computeFacts, countByRepo, SCAN_POLL_MS, useTrackerStore } from './tracker'

function mockFetch(handler: (url: string, init?: RequestInit) => Response | Promise<Response>) {
  const fn = vi.fn((url: string, init?: RequestInit) => Promise.resolve(handler(url, init)))
  vi.stubGlobal('fetch', fn)
  return fn
}
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })

describe('buildBlocks', () => {
  it('shows only unhealthy repos and merges alerts on the same target', () => {
    const blocks = buildBlocks(mixedState())
    expect(blocks.map((b) => b.repo.fullName)).toEqual(['o/r'])
    const rows = blocks[0].rows
    expect(rows).toHaveLength(2)
    const pr = rows.find((r) => r.url.endsWith('/pull/5'))!
    expect(pr.severity).toBe('crit')
    expect(pr.detail).toBe('open 9d · checks failing: build')
  })

  it('keeps the worst severity when a warning arrives first', () => {
    const s = state({
      repos: [repo({ health: 'crit' })],
      alerts: [
        alert({ severity: 'warn', url: 'u', detail: 'a' }),
        alert({ severity: 'crit', url: 'u', detail: 'b' }),
      ],
    })
    expect(buildBlocks(s)[0].rows[0].severity).toBe('crit')
  })

  it('orders rows by severity, critical first', () => {
    const s = state({
      repos: [repo({ health: 'crit' })],
      alerts: [
        alert({ severity: 'warn', url: 'w', subject: 'warn row' }),
        alert({ severity: 'crit', url: 'c', subject: 'crit row' }),
      ],
    })
    expect(buildBlocks(s)[0].rows.map((r) => r.subject)).toEqual(['crit row', 'warn row'])
  })

  it('keeps the server order for equal ages and ignores alerts for unknown repos', () => {
    const since = '2026-10-01T00:00:00Z'
    const s = state({
      repos: [repo({ fullName: 'o/b' }), repo({ fullName: 'o/a' })],
      alerts: [
        alert({ repo: 'o/a', url: '1', since }),
        alert({ repo: 'o/b', url: '2', since }),
        alert({ repo: 'o/ghost', url: '3', since }),
      ],
    })
    expect(buildBlocks(s).map((b) => b.repo.fullName)).toEqual(['o/b', 'o/a'])
  })

  it('puts the repo with the newest problem first, whatever its kind or severity', () => {
    const s = state({
      repos: [
        repo({ fullName: 'o/old-crit', health: 'crit' }),
        repo({ fullName: 'o/new-warn', health: 'warn' }),
        repo({ fullName: 'o/mid', health: 'crit' }),
      ],
      alerts: [
        alert({
          repo: 'o/old-crit',
          url: 'a',
          severity: 'crit',
          kind: 'pr_waiting',
          since: '2026-05-19T00:00:00Z',
        }),
        alert({
          repo: 'o/new-warn',
          url: 'b',
          severity: 'warn',
          kind: 'workflow_stuck',
          since: '2026-10-02T12:00:00Z',
        }),
        alert({
          repo: 'o/mid',
          url: 'c',
          severity: 'crit',
          kind: 'workflow_failed',
          since: '2026-09-20T00:00:00Z',
        }),
      ],
    })
    expect(buildBlocks(s).map((b) => b.repo.fullName)).toEqual([
      'o/new-warn',
      'o/mid',
      'o/old-crit',
    ])
  })

  it('ranks a repo by its newest alert, not its oldest', () => {
    const s = state({
      repos: [repo({ fullName: 'o/a', health: 'crit' }), repo({ fullName: 'o/b', health: 'crit' })],
      alerts: [
        alert({ repo: 'o/a', url: 'a1', since: '2026-01-01T00:00:00Z' }),
        alert({ repo: 'o/a', url: 'a2', since: '2026-10-02T00:00:00Z' }),
        alert({ repo: 'o/b', url: 'b1', since: '2026-06-01T00:00:00Z' }),
      ],
    })
    expect(buildBlocks(s).map((b) => b.repo.fullName)).toEqual(['o/a', 'o/b'])
  })

  it('returns no blocks when everything is healthy', () => {
    expect(buildBlocks(state({ repos: [repo()] }))).toEqual([])
  })
})

describe('overview helpers', () => {
  it('counts alerts of each severity per repo', () => {
    const counts = countByRepo(mixedState())
    expect(counts.get('o/r')).toEqual({ crit: 2, warn: 1 })
    expect(counts.has('o/fine')).toBe(false)
  })

  it('derives headline facts from repos and alert kinds', () => {
    const s = mixedState()
    s.alerts.push(
      alert({ kind: 'partial_data', severity: 'warn', url: 'x', subject: 'Incomplete data' }),
    )
    expect(computeFacts(s)).toEqual({
      failingWorkflows: 1,
      waitingPrs: 1,
      failingChecks: 1,
      staleIssues: 2,
      dataProblems: 1,
    })
  })
})

describe('tracker store', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => vi.unstubAllGlobals())

  it('loads state and derives blocks', async () => {
    mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    expect(store.error).toBe('')
    expect(store.state?.repos).toHaveLength(2)
    expect(store.blocks).toHaveLength(1)
    expect(store.loading).toBe(false)
  })

  it('keeps the last good state and reports the error when a poll fails', async () => {
    const fetchMock = mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    fetchMock.mockImplementationOnce(() => Promise.resolve(new Response('boom', { status: 500 })))
    await store.load()
    expect(store.error).toContain('HTTP 500')
    expect(store.state?.repos).toHaveLength(2)
  })

  it('reports network failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))
    const store = useTrackerStore()
    await store.load()
    expect(store.error).toBe('offline')
    expect(store.state).toBeNull()
  })

  it('toggles the focused repo and exposes filtered blocks and facets', async () => {
    mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    store.toggleRepo('o/fine')
    expect(store.filters.repo).toBe('o/fine')
    expect(store.visibleBlocks).toEqual([])
    store.toggleRepo('o/fine')
    expect(store.filters.repo).toBeNull()
    store.filters.severity = 'warn'
    expect(store.visibleBlocks).toEqual([])
    expect(store.facets.severity.all).toBe(store.facets.severity.crit + store.facets.severity.warn)
    store.filters.severity = 'crit'
    expect(store.visibleBlocks).toHaveLength(1)
    expect(store.activeFilters).toBe(1)
    expect(store.facts?.failingWorkflows).toBe(1)
    expect(store.owners).toEqual(['o'])
  })

  it('counts rows by severity so the headline matches the lists', async () => {
    mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    // three alerts, but the two on the same pull request merge into one row
    expect(store.rowCounts).toEqual({ crit: 2, warn: 0 })
    expect(store.facets.severity.all).toBe(store.rowCounts.crit + store.rowCounts.warn)
  })

  it('searches projects by name and clears every filter at once', async () => {
    mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    store.filters.query = 'fine'
    expect(store.visibleRepos.map((r) => r.fullName)).toEqual(['o/fine'])
    expect(store.visibleBlocks).toEqual([])
    store.clearFilters()
    expect(store.activeFilters).toBe(0)
    expect(store.visibleRepos).toHaveLength(2)
  })

  it('keeps a project in the list when the search matches one of its items', async () => {
    mockFetch(() => json(mixedState()))
    const store = useTrackerStore()
    await store.load()
    store.filters.query = 'bold'
    expect(store.visibleRepos.map((r) => r.fullName)).toEqual(['o/r'])
    store.filters.owners = ['other']
    expect(store.visibleRepos).toEqual([])
  })

  it('anchors the clock to server time so skewed browsers show right ages', async () => {
    mockFetch(() => json(state({ now: '2030-01-01T00:00:00Z' })))
    const store = useTrackerStore()
    await store.load()
    expect(Math.abs(store.nowMs - Date.parse('2030-01-01T00:00:00Z'))).toBeLessThan(2000)
  })

  it('posts a rescan and follows it until new data arrives', async () => {
    vi.useFakeTimers()
    try {
      const calls: string[] = []
      let polls = 0
      mockFetch((url, init) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        if (url === '/refresh') return new Response(null, { status: 202 })
        polls += 1
        // Poll 1: old data. Poll 2: scan running. Poll 3: finished with a new timestamp.
        if (polls === 1) return json(state({ lastRefresh: '2026-10-02T11:00:00Z' }))
        if (polls === 2)
          return json(state({ lastRefresh: '2026-10-02T11:00:00Z', refreshing: true }))
        return json(state({ lastRefresh: '2026-10-02T12:00:00Z' }))
      })
      const store = useTrackerStore()
      await store.load()
      const done = store.rescan()
      await vi.advanceTimersByTimeAsync(0)
      expect(store.scanning).toBe(true)
      await vi.advanceTimersByTimeAsync(SCAN_POLL_MS * 3)
      await done
      expect(store.scanning).toBe(false)
      expect(store.state?.lastRefresh).toBe('2026-10-02T12:00:00Z')
      expect(calls[0]).toBe('GET /api/state')
      expect(calls[1]).toBe('POST /refresh')
    } finally {
      vi.useRealTimers()
    }
  })

  it('shows a notice, not an error, when a rescan is rate limited', async () => {
    mockFetch(() => new Response('slow down', { status: 429 }))
    const store = useTrackerStore()
    await store.rescan()
    expect(store.notice).toContain('once every 5 minutes')
    expect(store.error).toBe('')
    expect(store.scanning).toBe(false)
  })

  it('reports other rescan failures as errors', async () => {
    mockFetch(() => new Response('boom', { status: 500 }))
    const store = useTrackerStore()
    await store.rescan()
    expect(store.error).toContain('HTTP 500')
    expect(store.notice).toBe('')
  })

  describe('refreshRepo', () => {
    const refreshed = (over = {}) => ({
      ...state({ repos: [repo({ refreshedAt: '2026-10-02T12:00:00Z', ...over })] }),
      repo: 'o/r',
    })

    it('posts the repo, swaps the state in and announces it without another fetch', async () => {
      const calls: string[] = []
      mockFetch((url, init) => {
        calls.push(`${init?.method ?? 'GET'} ${url}`)
        return json(refreshed())
      })
      const store = useTrackerStore()
      const done = store.refreshRepo('o/r')
      expect(store.refreshingRepos).toEqual(['o/r'])
      await done
      expect(calls).toEqual(['POST /refresh?repo=o%2Fr'])
      expect(store.refreshingRepos).toEqual([])
      expect(store.state?.repos[0].refreshedAt).toBe('2026-10-02T12:00:00Z')
      expect(store.announcement).toBe('Refreshed o/r')
      expect(store.notice).toBe('')
    })

    it('ignores a second request for the same repo while one is pending', async () => {
      const fetchMock = mockFetch(() => json(refreshed()))
      const store = useTrackerStore()
      await Promise.all([store.refreshRepo('o/r'), store.refreshRepo('o/r')])
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })

    it('shows the wait from Retry-After on 429', async () => {
      mockFetch(() => new Response('{}', { status: 429, headers: { 'Retry-After': '42' } }))
      const store = useTrackerStore()
      await store.refreshRepo('o/r')
      expect(store.notice).toBe('o/r was refreshed recently. Try again in 42s')
      expect(store.refreshingRepos).toEqual([])
      expect(store.error).toBe('')
    })

    it('says a scan is running on 409', async () => {
      mockFetch(() => new Response('{}', { status: 409 }))
      const store = useTrackerStore()
      await store.refreshRepo('o/r')
      expect(store.notice).toContain('already running')
    })

    it('reports other failures and a repo that came back down', async () => {
      mockFetch(() => new Response('{}', { status: 500 }))
      const store = useTrackerStore()
      await store.refreshRepo('o/r')
      expect(store.notice).toContain('HTTP 500')
      mockFetch(() => json(refreshed({ up: false, error: 'boom' })))
      await store.refreshRepo('o/r')
      expect(store.notice).toBe('Could not refresh o/r: boom')
      expect(store.announcement).toBe('')
    })
  })
})
