import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import AgeMeter from './AgeMeter.vue'
import AppHeader from './AppHeader.vue'
import AttentionList from './AttentionList.vue'
import FilterBar from './FilterBar.vue'
import LoadingScreen from './LoadingScreen.vue'
import AttentionRow from './AttentionRow.vue'
import FleetOverview from './FleetOverview.vue'
import ProjectList from './ProjectList.vue'
import RepoGroup from './RepoGroup.vue'
import { activeFilterCount, emptyFilters, facetCounts, type Filters } from '@/lib/filters'
import { buildBlocks, computeFacts, countByRepo } from '@/stores/tracker'
import { hoursAgo, mixedState, NOW, repo } from '@/test/fixtures'
import type { Row } from '@/lib/types'

const nowMs = Date.parse(NOW)
const CRIT_SECONDS = 604800
const text = (w: { text(): string }) => w.text().replace(/\s+/g, ' ')

describe('AppHeader', () => {
  const base = {
    lastRefresh: '2026-10-02T11:00:00Z',
    nowMs,
    intervalSeconds: 21600,
    loaded: true,
    scanning: false,
    progress: '',
  }

  it('shows when data was fetched and when the next refresh is due', () => {
    const w = mount(AppHeader, { props: { ...base, manualRefresh: false } })
    expect(text(w.get('[data-test=updated]'))).toBe('Scanned 1h ago · next in 5h')
    expect(w.find('[data-test=rescan]').exists()).toBe(false)
  })

  it('shows Loading before the first refresh finishes', () => {
    const w = mount(AppHeader, { props: { ...base, loaded: false, manualRefresh: false } })
    expect(w.text()).toContain('Loading')
  })

  it('emits rescan and shows progress while a scan runs', async () => {
    const w = mount(AppHeader, { props: { ...base, manualRefresh: true } })
    expect(w.get('[data-test=rescan]').text()).toContain('Rescan repositories')
    await w.get('[data-test=rescan]').trigger('click')
    expect(w.emitted('rescan')).toHaveLength(1)
    const busy = mount(AppHeader, { props: { ...base, manualRefresh: true, scanning: true } })
    expect(busy.get('[data-test=rescan]').attributes('disabled')).toBeDefined()
    expect(busy.get('[data-test=rescan]').text()).toContain('Scanning')
    const counted = mount(AppHeader, {
      props: { ...base, manualRefresh: true, scanning: true, progress: '23/63' },
    })
    expect(counted.get('[data-test=rescan]').text()).toContain('Scanning 23/63')
  })
})

describe('FleetOverview', () => {
  const s = mixedState()
  const props = {
    crit: 2,
    warn: 1,
    blockCount: 1,
    repos: s.repos,
    counts: countByRepo(s),
    facts: computeFacts(s),
    focused: null,
    matching: new Set(['o/r', 'o/fine']),
  }

  it('leads with the critical and warning counts and the scope', () => {
    const w = mount(FleetOverview, { props })
    const headline = w.get('[data-test=headline]').text()
    expect(headline).toMatch(/2\s*critical/)
    expect(headline).toMatch(/1\s*warning/)
    expect(w.get('[data-test=scope]').text()).toBe('1 of 2 projects need attention')
  })

  it('lists the headline facts, dimming zeros', () => {
    const w = mount(FleetOverview, { props })
    const facts = Object.fromEntries(
      w.findAll('[data-test=facts] > div').map((d) => [d.get('dt').text(), d.get('dd').text()]),
    )
    expect(facts).toMatchObject({
      'Workflows failing': '1',
      'Stale issues': '2',
      'Data problems': '0',
    })
  })

  it('says all clear when nothing is wrong', () => {
    const w = mount(FleetOverview, { props: { ...props, crit: 0, warn: 0, blockCount: 0 } })
    expect(w.get('[data-test=headline]').text()).toBe('All clear')
    expect(w.get('[data-test=scope]').text()).toBe('All 2 projects are healthy')
  })

  it('draws one segment per project, labelled for screen readers, and emits on click', async () => {
    const w = mount(FleetOverview, { props: { ...props, focused: 'o/r' } })
    const segments = w.findAll('[data-test=segment]')
    expect(segments).toHaveLength(2)
    expect(segments[0].attributes('aria-label')).toBe('o/r: 2 critical · 1 warning')
    expect(segments[0].attributes('aria-pressed')).toBe('true')
    expect(segments[1].attributes('aria-label')).toBe('o/fine: healthy')
    await segments[1].trigger('click')
    expect(w.emitted('select')?.[0]).toEqual(['o/fine'])
  })
})

describe('AgeMeter', () => {
  it('fills proportionally and always shows at least one segment', () => {
    const filled = (ratio: number) =>
      mount(AgeMeter, { props: { ratio, severity: 'crit' } })
        .findAll('span span')
        .filter((s) => s.classes().includes('bg-crit')).length
    expect(filled(0)).toBe(1)
    expect(filled(0.5)).toBe(5)
    expect(filled(3)).toBe(10)
  })

  it('uses the warning colour for warnings', () => {
    const w = mount(AgeMeter, { props: { ratio: 0.2, severity: 'warn' } })
    expect(w.find('.bg-warn').exists()).toBe(true)
    expect(w.find('.bg-crit').exists()).toBe(false)
  })
})

describe('AttentionRow', () => {
  const row = (over: Partial<Row> = {}): Row => ({
    repo: 'o/r',
    kind: 'pr_waiting',
    subject: '#5 <b>bold</b> title',
    detail: 'open 9d',
    url: 'https://github.com/o/r/pull/5',
    since: hoursAgo(216),
    severity: 'crit',
    ...over,
  })

  it('links the subject, shows the age and escapes markup', () => {
    const w = mount(AttentionRow, { props: { row: row(), nowMs, critSeconds: CRIT_SECONDS } })
    expect(w.get('a').attributes('href')).toBe('https://github.com/o/r/pull/5')
    expect(w.text()).toContain('<b>bold</b>')
    expect(w.find('b').exists()).toBe(false)
    expect(w.text()).toContain('9d')
    expect(w.text()).toContain('Critical')
  })

  it('shows the age meter only for waiting pull requests', () => {
    const pr = mount(AttentionRow, { props: { row: row(), nowMs, critSeconds: CRIT_SECONDS } })
    expect(pr.find('[aria-label$="critical age"]').exists()).toBe(true)
    const wf = mount(AttentionRow, {
      props: { row: row({ kind: 'workflow_failed' }), nowMs, critSeconds: CRIT_SECONDS },
    })
    expect(wf.find('[aria-label$="critical age"]').exists()).toBe(false)
  })

  it('omits the age for an unknown timestamp', () => {
    const w = mount(AttentionRow, {
      props: {
        row: row({ since: '0001-01-01T00:00:00Z', kind: 'issue_stale' }),
        nowMs,
        critSeconds: CRIT_SECONDS,
      },
    })
    expect(w.find('.font-mono').text()).toBe('')
  })
})

describe('RepoGroup', () => {
  const block = buildBlocks(mixedState())[0]

  it('shows the repo, its version link and an item count', () => {
    const w = mount(RepoGroup, { props: { block, nowMs, critSeconds: CRIT_SECONDS } })
    expect(w.text()).toContain('o/')
    expect(w.get('a[href="https://github.com/o/r"]').text()).toContain('r')
    expect(w.get('a[href="https://github.com/o/r/releases/tag/v1.0.0"]').text()).toBe('v1.0.0')
    expect(w.text()).toContain('2 items')
    expect(w.findAll('[data-test=row]')).toHaveLength(2)
  })

  it('omits the version badge without a release and uses the singular for one item', () => {
    const one = { repo: repo({ release: null }), rows: [block.rows[0]] }
    const w = mount(RepoGroup, { props: { block: one, nowMs, critSeconds: CRIT_SECONDS } })
    expect(w.find('a[href*="/releases/"]').exists()).toBe(false)
    expect(w.text()).toContain('1 item')
  })
})

describe('AttentionList', () => {
  const blocks = buildBlocks(mixedState())
  const props = {
    blocks,
    nowMs,
    critSeconds: CRIT_SECONDS,
    facets: facetCounts(blocks, emptyFilters(), ['o']),
    owners: ['o'],
    active: 0,
    filters: emptyFilters(),
  }

  it('renders one group per repo', () => {
    const w = mount(AttentionList, { props })
    expect(w.findAll('[data-test=repo-block]')).toHaveLength(1)
  })

  it('bubbles filter changes up', async () => {
    const w = mount(AttentionList, { props })
    await w.get('[data-test=sev-crit]').trigger('click')
    const [emitted] = w.emitted('update:filters')!.at(-1) as [Filters]
    expect(emitted.severity).toBe('crit')
  })

  it('forwards clear', async () => {
    const w = mount(AttentionList, { props: { ...props, active: 2 } })
    await w.get('[data-test=clear-all]').trigger('click')
    expect(w.emitted('clear')).toHaveLength(1)
  })

  it('explains an empty selection', () => {
    const w = mount(AttentionList, { props: { ...props, blocks: [] } })
    expect(w.get('[data-test=empty]').text()).toContain('Nothing matches')
  })
})

describe('FilterBar', () => {
  const blocks = buildBlocks(mixedState())
  const base = (filters: Filters = emptyFilters(), owners = ['o', 'p']) => ({
    facets: facetCounts(blocks, filters, owners),
    owners,
    active: activeFilterCount(filters),
    filters,
  })

  it('shows live counts in every facet', () => {
    const w = mount(FilterBar, { props: base() })
    expect(w.get('[data-test=sev-all]').text()).toBe('All 2')
    expect(w.get('[data-test=sev-crit]').text()).toBe('Critical 2')
    expect(w.get('[data-test=sev-warn]').text()).toBe('Warnings 0')
    expect(w.get('[data-test=kind-workflow]').text()).toContain('1')
    expect(w.get('[data-test=kind-pr]').text()).toContain('1')
    expect(w.get('[data-test=owner-o]').text()).toContain('2')
  })

  it('disables facet values that would match nothing, but not selected ones', () => {
    const w = mount(FilterBar, { props: base() })
    expect(w.get('[data-test=kind-issue]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test=owner-p]').attributes('disabled')).toBeDefined()
    expect(w.get('[data-test=kind-pr]').attributes('disabled')).toBeUndefined()
    const sel = mount(FilterBar, { props: base({ ...emptyFilters(), kinds: ['issue'] }) })
    expect(sel.get('[data-test=kind-issue]').attributes('disabled')).toBeUndefined()
  })

  it('narrows the other facets when one is selected', () => {
    const w = mount(FilterBar, { props: base({ ...emptyFilters(), kinds: ['workflow'] }) })
    expect(w.get('[data-test=sev-all]').text()).toBe('All 1')
    expect(w.get('[data-test=kind-pr]').text()).toContain('1')
  })

  it('hides the owner filter when there is only one owner', () => {
    const w = mount(FilterBar, { props: base(emptyFilters(), ['o']) })
    expect(w.find('[data-test=owner-o]').exists()).toBe(false)
  })

  it('emits typed search text and clears it with Escape', async () => {
    const w = mount(FilterBar, { props: base() })
    const input = w.get('[data-test=search]')
    await input.setValue('grpc')
    expect((w.emitted('update:filters')!.at(-1) as [Filters])[0].query).toBe('grpc')
    const typed = mount(FilterBar, { props: base({ ...emptyFilters(), query: 'grpc' }) })
    await typed.get('[data-test=search]').trigger('keydown', { key: 'Escape' })
    expect((typed.emitted('update:filters')!.at(-1) as [Filters])[0].query).toBe('')
  })

  it('offers to clear active filters with a count', async () => {
    const f = { ...emptyFilters(), query: 'x', kinds: ['pr' as const] }
    const w = mount(FilterBar, { props: base(f) })
    expect(w.get('[data-test=clear-all]').text()).toBe('Clear 2 filters')
    await w.get('[data-test=clear-all]').trigger('click')
    expect(w.emitted('clear')).toHaveLength(1)
    expect(mount(FilterBar, { props: base() }).find('[data-test=clear-all]').exists()).toBe(false)
  })
})

describe('ProjectList', () => {
  const repos = mixedState().repos

  it('lists every project with release, counts and links', () => {
    const w = mount(ProjectList, { props: { repos, total: 2, focused: null, nowMs } })
    const rows = w.findAll('[data-test=project]')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('v1.0.0')
    expect(rows[0].text()).toContain('released 3d ago')
    expect(rows[0].text()).toContain('1/3 failing')
    expect(rows[0].get('a[href="https://github.com/o/r/pulls"]').text()).toContain('2')
    expect(rows[0].get('a[href="https://github.com/o/r/issues"]').text()).toContain('4')
    expect(rows[1].get('a[href="https://github.com/o/fine/releases"]').text()).toBe('no release')
  })

  it('shows how many projects match and a hint when none do', () => {
    const some = mount(ProjectList, {
      props: { repos: repos.slice(0, 1), total: 2, focused: null, nowMs },
    })
    expect(some.get('[data-test=project-count]').text()).toBe('1 of 2')
    const none = mount(ProjectList, { props: { repos: [], total: 2, focused: null, nowMs } })
    expect(none.find('[data-test=no-projects]').exists()).toBe(true)
  })

  it('emits the repo when its name is chosen and highlights the focused one', async () => {
    const w = mount(ProjectList, { props: { repos, total: 2, focused: 'o/fine', nowMs } })
    await w.findAll('[data-test=project] button')[0].trigger('click')
    expect(w.emitted('select')?.[0]).toEqual(['o/r'])
    expect(w.findAll('[data-test=project]')[1].classes()).toContain('bg-accent')
  })
})

describe('AttentionList columns', () => {
  const blocks = ['a/one', 'b/two', 'c/three'].map((fullName) => ({
    repo: repo({ fullName, health: 'crit' }),
    rows: buildBlocks(mixedState())[0].rows,
  }))
  const props = {
    blocks,
    nowMs,
    critSeconds: CRIT_SECONDS,
    facets: facetCounts(blocks, emptyFilters(), ['a', 'b', 'c']),
    owners: ['a'],
    active: 0,
    filters: emptyFilters(),
  }

  afterEach(() => vi.unstubAllGlobals())

  it('uses one column on narrow screens', () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: false,
      addEventListener: () => {},
      removeEventListener: () => {},
    }))
    const w = mount(AttentionList, { props })
    expect(w.findAll('[data-test=columns] > div')).toHaveLength(1)
  })

  it('balances groups over two columns on very wide screens', async () => {
    vi.stubGlobal('matchMedia', () => ({
      matches: true,
      addEventListener: () => {},
      removeEventListener: () => {},
    }))
    const w = mount(AttentionList, { props })
    await w.vm.$nextTick()
    const cols = w.findAll('[data-test=columns] > div')
    expect(cols).toHaveLength(2)
    expect(cols[0].findAll('[data-test=repo-block]')).toHaveLength(2)
    expect(cols[1].findAll('[data-test=repo-block]')).toHaveLength(1)
  })
})

describe('ProjectList lazy loading', () => {
  type Callback = (entries: { isIntersecting: boolean }[]) => void
  let callback: Callback = () => {}

  function stubObserver() {
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        constructor(cb: Callback) {
          callback = cb
        }
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    )
  }
  const many = (n: number) => Array.from({ length: n }, (_, i) => repo({ fullName: `o/repo-${i}` }))

  afterEach(() => vi.unstubAllGlobals())

  it('renders a first page, then more each time the end marker scrolls into view', async () => {
    stubObserver()
    const w = mount(ProjectList, { props: { repos: many(55), total: 55, focused: null, nowMs } })
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(20)
    expect(w.get('[data-test=more]').text()).toBe('Showing 20 of 55')
    callback([{ isIntersecting: true }])
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(40)
    callback([{ isIntersecting: false }])
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(40)
    callback([{ isIntersecting: true }])
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(55)
    expect(w.find('[data-test=more]').exists()).toBe(false)
  })

  it('starts over from the first page when the list changes', async () => {
    stubObserver()
    const w = mount(ProjectList, { props: { repos: many(55), total: 55, focused: null, nowMs } })
    await w.vm.$nextTick()
    callback([{ isIntersecting: true }])
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(40)
    await (w as unknown as { setProps(p: object): Promise<void> }).setProps({ repos: many(30) })
    expect(w.findAll('[data-test=project]')).toHaveLength(20)
  })

  it('renders everything when the browser has no IntersectionObserver', async () => {
    vi.stubGlobal('IntersectionObserver', undefined)
    const w = mount(ProjectList, { props: { repos: many(55), total: 55, focused: null, nowMs } })
    await w.vm.$nextTick()
    await w.vm.$nextTick()
    expect(w.findAll('[data-test=project]')).toHaveLength(55)
  })
})

describe('LoadingScreen', () => {
  it('shows scan progress with a count once the server reports a total', () => {
    const w = mount(LoadingScreen, { props: { done: 23, total: 63, connecting: false } })
    expect(w.get('[data-test=loading-title]').text()).toBe('Scanning repositories')
    expect(w.get('[data-test=loading-detail]').text()).toContain('23 of 63 scanned')
    expect(w.get('[role=progressbar]').attributes('aria-valuenow')).toBe('37')
  })

  it('draws one skeleton cell per repo, within sensible limits', () => {
    const cells = (total: number) =>
      mount(LoadingScreen, { props: { done: 0, total, connecting: false } }).findAll(
        '.grid-cols-\\[repeat\\(auto-fill\\,minmax\\(1\\.5rem\\,1fr\\)\\)\\] > div',
      ).length
    expect(cells(5)).toBe(12)
    expect(cells(30)).toBe(30)
    expect(cells(500)).toBe(63)
  })

  it('says it is connecting before the first response', () => {
    const w = mount(LoadingScreen, { props: { done: 0, total: 0, connecting: true } })
    expect(w.get('[data-test=loading-title]').text()).toBe('Connecting')
    expect(w.find('[role=progressbar]').exists()).toBe(false)
  })

  it('uses an indeterminate bar while the total is not known yet', () => {
    const w = mount(LoadingScreen, { props: { done: 0, total: 0, connecting: false } })
    expect(w.get('[data-test=loading-detail]').text()).toContain('Contacting GitHub')
    expect(w.find('[role=progressbar]').exists()).toBe(false)
  })

  it('is announced politely to screen readers', () => {
    const w = mount(LoadingScreen, { props: { done: 1, total: 2, connecting: false } })
    expect(w.attributes('role')).toBe('status')
    expect(w.attributes('aria-live')).toBe('polite')
  })
})

describe('per-repository refresh button', () => {
  const block = buildBlocks(mixedState())[0]
  const repos = mixedState().repos
  const group = (extra = {}) =>
    mount(RepoGroup, { props: { block, nowMs, critSeconds: CRIT_SECONDS, ...extra } })
  const list = (extra = {}) =>
    mount(ProjectList, { props: { repos, total: 2, focused: null, nowMs, ...extra } })

  it('is hidden unless manual refresh is on', () => {
    expect(group().find('[data-test=repo-refresh]').exists()).toBe(false)
    expect(list().find('[data-test=repo-refresh]').exists()).toBe(false)
    expect(list({ manualRefresh: false }).find('[data-test=repo-refresh]').exists()).toBe(false)
  })

  it('renders one labelled button per project and one per group header', () => {
    const l = list({ manualRefresh: true })
    const buttons = l.findAll('[data-test=project] [data-test=repo-refresh]')
    expect(buttons.map((b) => b.attributes('aria-label'))).toEqual([
      'Refresh o/r',
      'Refresh o/fine',
    ])
    const g = group({ manualRefresh: true })
    expect(g.get('header [data-test=repo-refresh]').attributes('aria-label')).toBe('Refresh o/r')
  })

  it('emits the repo name on click without selecting the project', async () => {
    const l = list({ manualRefresh: true })
    await l.findAll('[data-test=repo-refresh]')[1].trigger('click')
    expect(l.emitted('refresh')).toEqual([['o/fine']])
    expect(l.emitted('select')).toBeUndefined()
    const g = group({ manualRefresh: true })
    await g.get('[data-test=repo-refresh]').trigger('click')
    expect(g.emitted('refresh')).toEqual([['o/r']])
  })

  it('disables only the refreshing repo and spins its icon', () => {
    const l = list({ manualRefresh: true, refreshing: ['o/r'] })
    const [busy, idle] = l.findAll('[data-test=repo-refresh]')
    expect(busy.attributes('disabled')).toBeDefined()
    expect(busy.attributes('aria-busy')).toBe('true')
    expect(busy.find('svg').classes()).toContain('fa-spin')
    expect(idle.attributes('disabled')).toBeUndefined()
    expect(idle.find('svg').classes()).not.toContain('fa-spin')
  })

  it('disables every button while a full scan runs', () => {
    const l = list({ manualRefresh: true, scanning: true })
    for (const b of l.findAll('[data-test=repo-refresh]'))
      expect(b.attributes('disabled')).toBeDefined()
  })
})
