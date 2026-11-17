// Serves a built dist/ with a fixed /api/state, so the page renders without a GitHub token.
// Usage: node .claude/skills/validating-ui-components/mock-server.mjs <dist-dir> [port]
import { existsSync, readFileSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, resolve } from 'node:path'

const DIST = resolve(process.argv[2] ?? 'dist')
const PORT = Number(process.argv[3] ?? 5288)
const now = Date.now()
const ago = (h) => new Date(now - h * 3600_000).toISOString()

// 28 repos give a full fleet strip, every health state and enough rows to scroll.
const names = [
  'api',
  'web',
  'worker',
  'billing',
  'auth',
  'search',
  'gateway',
  'infra',
  'docs',
  'cli',
  'sdk-go',
  'sdk-ts',
  'mailer',
  'scheduler',
  'metrics',
  'ingest',
  'exporter',
  'proxy',
  'cache',
  'queue',
  'notifier',
  'reports',
  'admin',
  'mobile',
  'design',
  'charts',
  'deploy',
  'backup',
]
const health = (i) => (i % 9 === 0 ? 'crit' : i % 5 === 0 ? 'warn' : 'ok')

const repos = names.map((n, i) => ({
  fullName: `${i % 3 ? 'acme' : 'tools'}/${n}`,
  url: `https://github.com/acme/${n}`,
  releasesUrl: `https://github.com/acme/${n}/releases`,
  description: '',
  health: health(i),
  error: '',
  workflows: { total: 3, failing: health(i) === 'crit' ? 1 : 0, running: 0 },
  prs: { open: i % 4, waiting: 0, failingChecks: 0, oldestSeconds: 0 },
  issues: { open: i % 7, stale: 0 },
  archived: false,
  up: true,
  refreshedAt: ago(1),
  release:
    i % 6 === 5
      ? null
      : {
          tag: `v1.${i}.0`,
          url: `https://github.com/acme/${n}/releases/tag/v1.${i}.0`,
          publishedAt: ago(24 * (i + 1)),
          prerelease: false,
          fromTag: false,
        },
}))

const alerts = []
repos.forEach((r, i) => {
  if (r.health === 'crit') {
    alerts.push({
      repo: r.fullName,
      kind: 'workflow_failed',
      severity: 'crit',
      subject: 'CI',
      detail: 'failure on main (push)',
      url: `${r.url}/actions/runs/${i}`,
      since: ago(3),
    })
    alerts.push({
      repo: r.fullName,
      kind: 'pr_waiting',
      severity: 'crit',
      subject: `#${10 + i} Bump dependencies to latest`,
      detail: 'open 9d, no review',
      url: `${r.url}/pull/${10 + i}`,
      since: ago(216),
    })
  }
  if (r.health === 'warn') {
    alerts.push({
      repo: r.fullName,
      kind: 'pr_waiting',
      severity: 'warn',
      subject: `#${30 + i} Add retry on upstream timeout`,
      detail: 'open 3d',
      url: `${r.url}/pull/${30 + i}`,
      since: ago(72),
    })
    alerts.push({
      repo: r.fullName,
      kind: 'issue_stale',
      severity: 'warn',
      subject: `#${50 + i} Flaky integration test`,
      detail: 'no activity for 45d',
      url: `${r.url}/issues/${50 + i}`,
      since: ago(24 * 45),
    })
  }
})

// Must match TrackerState in src/lib/types.ts.
const state = {
  now: new Date(now).toISOString(),
  lastRefresh: ago(1),
  intervalSeconds: 21600,
  loaded: true,
  refreshing: false,
  manualRefresh: true,
  scan: { done: 0, total: 0 },
  counts: {
    crit: alerts.filter((a) => a.severity === 'crit').length,
    warn: alerts.filter((a) => a.severity === 'warn').length,
  },
  thresholds: {
    prWarnSeconds: 172800,
    prCritSeconds: 604800,
    issueStaleSeconds: 2592000,
    workflowStuckSeconds: 21600,
    workflowDormantSeconds: 2592000,
  },
  repos,
  alerts,
  actions: [
    {
      id: 'rerun',
      label: 'Re-run failed jobs',
      confirm: 'Re-run the failed jobs of this workflow?',
      kinds: ['workflow_failed'],
    },
  ],
}

const TYPES = {
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.css': 'text/css',
  '.woff2': 'font/woff2',
  '.svg': 'image/svg+xml',
}

createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost')
  if (url.pathname === '/api/state') {
    res.writeHead(200, { 'content-type': 'application/json' })
    return res.end(JSON.stringify(state))
  }
  let file = join(DIST, url.pathname === '/' ? 'index.html' : url.pathname)
  if (!file.startsWith(DIST) || !existsSync(file)) file = join(DIST, 'index.html')
  res.writeHead(200, { 'content-type': TYPES[extname(file)] ?? 'application/octet-stream' })
  res.end(readFileSync(file))
}).listen(PORT, '127.0.0.1', () => console.log(`mock on http://127.0.0.1:${PORT}`))
