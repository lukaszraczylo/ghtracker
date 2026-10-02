import type { Block, Repo, Row } from './types'

export type SeverityFilter = 'all' | 'crit' | 'warn'
export type KindGroup = 'workflow' | 'pr' | 'issue' | 'data'

export interface Filters {
  query: string
  severity: SeverityFilter
  kinds: KindGroup[]
  owners: string[]
  repo: string | null
}

export const KIND_GROUPS: KindGroup[] = ['workflow', 'pr', 'issue', 'data']

export const emptyFilters = (): Filters => ({
  query: '',
  severity: 'all',
  kinds: [],
  owners: [],
  repo: null,
})

export function kindGroup(kind: string): KindGroup {
  if (kind.startsWith('workflow_')) return 'workflow'
  if (kind.startsWith('pr_')) return 'pr'
  if (kind === 'issues_stale') return 'issue'
  return 'data'
}

export const ownerOf = (fullName: string): string => fullName.split('/')[0]

/** Every whitespace-separated term must appear somewhere in the haystack. */
export function matchesQuery(query: string, ...fields: string[]): boolean {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true
  const haystack = fields.join(' ').toLowerCase()
  return terms.every((t) => haystack.includes(t))
}

/** Facets a row or block can be counted under; the named one is ignored while counting it. */
export type Facet = 'severity' | 'kinds' | 'owners'

function rowMatches(repo: Repo, row: Row, f: Filters, skip?: Facet): boolean {
  if (f.repo && repo.fullName !== f.repo) return false
  if (skip !== 'severity' && f.severity !== 'all' && row.severity !== f.severity) return false
  if (skip !== 'kinds' && f.kinds.length > 0 && !f.kinds.includes(kindGroup(row.kind))) return false
  if (skip !== 'owners' && f.owners.length > 0 && !f.owners.includes(ownerOf(repo.fullName)))
    return false
  return matchesQuery(f.query, repo.fullName, repo.release?.tag ?? '', row.subject, row.detail)
}

export function applyFilters(blocks: Block[], f: Filters, skip?: Facet): Block[] {
  const out: Block[] = []
  for (const b of blocks) {
    const rows = b.rows.filter((r) => rowMatches(b.repo, r, f, skip))
    if (rows.length > 0) out.push({ repo: b.repo, rows })
  }
  return out
}

export interface Facets {
  severity: Record<SeverityFilter, number>
  kinds: Record<KindGroup, number>
  owners: Record<string, number>
}

/** Counts rows per facet value, each with every other filter applied (faceted search). */
export function facetCounts(blocks: Block[], f: Filters, owners: string[]): Facets {
  const rows = (skip: Facet) =>
    applyFilters(blocks, f, skip).flatMap((b) => b.rows.map((r) => ({ b, r })))
  const sev = rows('severity')
  const kinds = rows('kinds')
  const own = rows('owners')
  return {
    severity: {
      all: sev.length,
      crit: sev.filter(({ r }) => r.severity === 'crit').length,
      warn: sev.filter(({ r }) => r.severity === 'warn').length,
    },
    kinds: Object.fromEntries(
      KIND_GROUPS.map((g) => [g, kinds.filter(({ r }) => kindGroup(r.kind) === g).length]),
    ) as Facets['kinds'],
    owners: Object.fromEntries(
      owners.map((o) => [o, own.filter(({ b }) => ownerOf(b.repo.fullName) === o).length]),
    ),
  }
}

/** Repos that pass the query and owner filters; used for the project list and the strip. */
export function repoMatches(repo: Repo, f: Filters): boolean {
  if (f.owners.length > 0 && !f.owners.includes(ownerOf(repo.fullName))) return false
  return matchesQuery(f.query, repo.fullName, repo.release?.tag ?? '', repo.description)
}

export function activeFilterCount(f: Filters): number {
  return (
    (f.query.trim() ? 1 : 0) +
    (f.severity !== 'all' ? 1 : 0) +
    f.kinds.length +
    f.owners.length +
    (f.repo ? 1 : 0)
  )
}

/** Serialises filters for the URL; defaults are omitted so the plain URL stays clean. */
export function filtersToSearch(f: Filters): string {
  const p = new URLSearchParams()
  if (f.query.trim()) p.set('q', f.query.trim())
  if (f.severity !== 'all') p.set('sev', f.severity)
  if (f.kinds.length) p.set('kind', f.kinds.join(','))
  if (f.owners.length) p.set('owner', f.owners.join(','))
  if (f.repo) p.set('repo', f.repo)
  const s = p.toString()
  return s ? `?${s}` : ''
}

export function filtersFromSearch(search: string): Filters {
  const p = new URLSearchParams(search)
  const sev = p.get('sev')
  const list = (key: string) => (p.get(key) ?? '').split(',').filter(Boolean)
  return {
    query: p.get('q') ?? '',
    severity: sev === 'crit' || sev === 'warn' ? sev : 'all',
    kinds: list('kind').filter((k): k is KindGroup => (KIND_GROUPS as string[]).includes(k)),
    owners: list('owner'),
    repo: p.get('repo'),
  }
}
