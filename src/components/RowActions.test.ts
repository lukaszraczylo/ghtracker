import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import AttentionRow from './AttentionRow.vue'
import { alert, hoursAgo, repo, state } from '@/test/fixtures'
import { buildBlocks } from '@/stores/tracker'
import type { ActionDef, Alert, Row } from '@/lib/types'

const rerun: ActionDef = {
  id: 'rerun',
  label: 'Re-run',
  confirm: 'Run this action?',
  kinds: ['workflow_failed'],
}

const rowFor = (actions: ActionDef[], over: Partial<Alert> = {}): Row =>
  buildBlocks(state({ repos: [repo()], alerts: [alert(over)], actions }))[0].rows[0]

describe('buildBlocks with actions', () => {
  it('leaves rows untouched when no actions are configured', () => {
    const r = rowFor([])
    expect('actions' in r).toBe(false)
    expect('actionStatus' in r).toBe(false)
  })

  it.each([
    ['kind listed', ['workflow_failed'], true],
    ['kind not listed', ['pr_waiting'], false],
    ['empty kinds match everything', [], true],
  ])('%s', (_name, kinds, shown) => {
    const r = rowFor([{ ...rerun, kinds }])
    expect(Boolean(r.actions?.length)).toBe(shown)
  })

  it('carries the status of the alert', () => {
    const r = rowFor([rerun], { actionStatus: { rerun: { state: 'running' } } })
    expect(r.actionStatus).toEqual({ rerun: { state: 'running' } })
  })
})

describe('RowActions', () => {
  beforeEach(() => setActivePinia(createPinia()))
  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  const mountRow = (row: Row) =>
    mount(AttentionRow, {
      props: { row, nowMs: Date.parse(hoursAgo(0)), critSeconds: 604800 },
      attachTo: document.body,
    })

  it('shows no button without actions', () => {
    const w = mountRow(rowFor([]))
    expect(w.find('[data-test=row-actions]').exists()).toBe(false)
  })

  it('shows a button labelled from the config', () => {
    const w = mountRow(rowFor([rerun]))
    expect(w.find('[data-test=action-rerun]').text()).toBe('Re-run')
    expect(w.find('[data-test=action-status]').exists()).toBe(false)
  })

  it('asks for confirmation, then posts the alert', async () => {
    const fetchMock = vi.fn((url: string) =>
      Promise.resolve(
        new Response(
          JSON.stringify(
            url.startsWith('/api/actions/')
              ? { state: 'queued', link: 'https://ci.example.test/9' }
              : state(),
          ),
          { status: 200 },
        ),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const w = mountRow(rowFor([rerun]))
    await w.get('[data-test=action-rerun]').trigger('click')
    expect(document.body.textContent).toContain('Run this action?')
    expect(fetchMock).not.toHaveBeenCalled()
    ;(document.body.querySelector('[data-test=action-confirm]') as HTMLElement).click()
    await vi.waitFor(() => expect(w.find('[data-test=action-status]').exists()).toBe(true))
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(url).toBe('/api/actions/rerun')
    expect(JSON.parse(init.body as string)).toEqual({
      repo: 'o/r',
      url: 'https://github.com/o/r/actions/runs/1',
    })
    await vi.waitFor(() =>
      expect(w.get('[data-test=action-status] a').attributes('href')).toBe(
        'https://ci.example.test/9',
      ),
    )
  })

  it('posts at once when the action has no confirm text', async () => {
    const fetchMock = vi.fn<(url: string, init?: RequestInit) => Promise<Response>>(() =>
      Promise.resolve(new Response('{}', { status: 200 })),
    )
    vi.stubGlobal('fetch', fetchMock)
    const w = mountRow(rowFor([{ ...rerun, confirm: undefined }]))
    await w.get('[data-test=action-rerun]').trigger('click')
    expect(fetchMock).toHaveBeenCalledWith('/api/actions/rerun', expect.anything())
  })

  it('shows the status reported by the server as text, or as a link', () => {
    const plain = mountRow(
      rowFor([rerun], { actionStatus: { rerun: { state: 'done', label: 'Finished' } } }),
    )
    expect(plain.get('[data-test=action-status]').text()).toBe('Finished')
    expect(plain.find('[data-test=action-status] a').exists()).toBe(false)
    const linked = mountRow(
      rowFor([rerun], {
        actionStatus: { rerun: { state: 'done', link: 'https://x.example.test/' } },
      }),
    )
    expect(linked.get('[data-test=action-status] a').text()).toBe('done')
  })
})
