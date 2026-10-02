import { describe, expect, it } from 'vitest'
import {
  activeFilterCount,
  applyFilters,
  emptyFilters,
  facetCounts,
  filtersFromSearch,
  filtersToSearch,
  kindGroup,
  matchesQuery,
  repoMatches,
  type Filters,
} from './filters'
import { buildBlocks } from '@/stores/tracker'
import { alert, repo, state } from '@/test/fixtures'

const fleet = () =>
  buildBlocks(
    state({
      repos: [
        repo({ fullName: 'a/one', health: 'crit' }),
        repo({ fullName: 'b/two', health: 'warn', release: null }),
      ],
      alerts: [
        alert({
          repo: 'a/one',
          kind: 'workflow_failed',
          severity: 'crit',
          subject: 'CI',
          url: '1',
        }),
        alert({
          repo: 'a/one',
          kind: 'pr_waiting',
          severity: 'warn',
          subject: '#3 Bump grpc',
          url: '2',
        }),
        alert({
          repo: 'b/two',
          kind: 'issues_stale',
          severity: 'warn',
          subject: '4 stale issues',
          url: '3',
        }),
        alert({
          repo: 'b/two',
          kind: 'partial_data',
          severity: 'warn',
          subject: 'Incomplete data',
          url: '4',
        }),
      ],
    }),
  )

const withFilters = (over: Partial<Filters>): Filters => ({ ...emptyFilters(), ...over })

describe('kindGroup', () => {
  it.each([
    ['workflow_failed', 'workflow'],
    ['workflow_stuck', 'workflow'],
    ['pr_waiting', 'pr'],
    ['pr_checks_failing', 'pr'],
    ['issues_stale', 'issue'],
    ['refresh_failed', 'data'],
    ['data_stale', 'data'],
    ['partial_data', 'data'],
    ['something_new', 'data'],
  ])('%s -> %s', (kind, group) => expect(kindGroup(kind)).toBe(group))
})

describe('matchesQuery', () => {
  it('requires every term, ignores case and accepts an empty query', () => {
    expect(matchesQuery('', 'anything')).toBe(true)
    expect(matchesQuery('  ', 'anything')).toBe(true)
    expect(matchesQuery('bump GRPC', 'o/r', '#3 Bump google.golang.org/grpc')).toBe(true)
    expect(matchesQuery('bump nothing', '#3 Bump grpc')).toBe(false)
  })
})

describe('applyFilters', () => {
  it('returns everything for empty filters', () => {
    expect(applyFilters(fleet(), emptyFilters()).flatMap((b) => b.rows)).toHaveLength(4)
  })

  it('combines severity, kind, owner and search', () => {
    const rows = (f: Filters) =>
      applyFilters(fleet(), f).flatMap((b) => b.rows.map((r) => r.subject))
    expect(rows(withFilters({ severity: 'crit' }))).toEqual(['CI'])
    expect(rows(withFilters({ kinds: ['pr', 'issue'] }))).toEqual([
      '#3 Bump grpc',
      '4 stale issues',
    ])
    expect(rows(withFilters({ owners: ['b'] }))).toEqual(['4 stale issues', 'Incomplete data'])
    expect(rows(withFilters({ query: 'grpc' }))).toEqual(['#3 Bump grpc'])
    expect(rows(withFilters({ query: 'a/one', kinds: ['pr'] }))).toEqual(['#3 Bump grpc'])
    expect(rows(withFilters({ repo: 'b/two', severity: 'crit' }))).toEqual([])
  })

  it('drops blocks left without rows', () => {
    expect(
      applyFilters(fleet(), withFilters({ kinds: ['workflow'] })).map((b) => b.repo.fullName),
    ).toEqual(['a/one'])
  })

  it('also searches the release tag', () => {
    expect(
      applyFilters(fleet(), withFilters({ query: 'v1.0.0' })).map((b) => b.repo.fullName),
    ).toEqual(['a/one'])
  })
})

describe('facetCounts', () => {
  it('counts each facet with the others applied, but not itself', () => {
    const f = withFilters({ kinds: ['pr'] })
    const c = facetCounts(fleet(), f, ['a', 'b'])
    expect(c.kinds).toEqual({ workflow: 1, pr: 1, issue: 1, data: 1 })
    expect(c.severity).toEqual({ all: 1, crit: 0, warn: 1 })
    expect(c.owners).toEqual({ a: 1, b: 0 })
  })

  it('counts everything for empty filters', () => {
    const c = facetCounts(fleet(), emptyFilters(), ['a', 'b'])
    expect(c.severity).toEqual({ all: 4, crit: 1, warn: 3 })
    expect(c.owners).toEqual({ a: 2, b: 2 })
  })
})

describe('repoMatches', () => {
  it('uses the query and owner filters only', () => {
    const r = repo({ fullName: 'a/one', description: 'Kubernetes mirror' })
    expect(repoMatches(r, withFilters({ query: 'mirror' }))).toBe(true)
    expect(repoMatches(r, withFilters({ query: 'nope' }))).toBe(false)
    expect(repoMatches(r, withFilters({ owners: ['b'] }))).toBe(false)
    expect(repoMatches(r, withFilters({ severity: 'crit', kinds: ['pr'] }))).toBe(true)
  })
})

describe('activeFilterCount', () => {
  it('counts each active filter value', () => {
    expect(activeFilterCount(emptyFilters())).toBe(0)
    expect(
      activeFilterCount(
        withFilters({
          query: ' x ',
          severity: 'warn',
          kinds: ['pr', 'issue'],
          owners: ['a'],
          repo: 'a/b',
        }),
      ),
    ).toBe(6)
    expect(activeFilterCount(withFilters({ query: '   ' }))).toBe(0)
  })
})

describe('URL round trip', () => {
  it('omits defaults', () => {
    expect(filtersToSearch(emptyFilters())).toBe('')
  })

  it('serialises and restores every filter', () => {
    const f = withFilters({
      query: 'bump grpc',
      severity: 'crit',
      kinds: ['pr', 'data'],
      owners: ['a', 'b'],
      repo: 'a/one',
    })
    expect(filtersFromSearch(filtersToSearch(f))).toEqual(f)
  })

  it('ignores unknown values from a hand-edited URL', () => {
    expect(filtersFromSearch('?sev=nope&kind=pr,bogus&owner=,a')).toEqual(
      withFilters({ kinds: ['pr'], owners: ['a'] }),
    )
  })
})
