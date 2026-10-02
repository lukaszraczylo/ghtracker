import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'
import App from './App.vue'
import { mixedState, repo, state } from '@/test/fixtures'
import type { TrackerState } from '@/lib/types'

function mountWith(body: TrackerState | Error) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      body instanceof Error
        ? Promise.reject(body)
        : Promise.resolve(new Response(JSON.stringify(body), { status: 200 })),
    ),
  )
  return mount(App, { global: { plugins: [createPinia()] } })
}

describe('App', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('shows the overview, only the problems and every project', async () => {
    const w = mountWith(mixedState())
    await flushPromises()
    expect(w.get('[data-test=headline]').text()).toContain('2')
    expect(w.findAll('[data-test=segment]')).toHaveLength(2)
    expect(w.findAll('[data-test=repo-block]')).toHaveLength(1)
    expect(w.findAll('[data-test=row]')).toHaveLength(2)
    expect(w.findAll('[data-test=project]')).toHaveLength(2)
  })

  it('shows an all-clear state without repo blocks', async () => {
    const w = mountWith(state({ repos: [repo()] }))
    await flushPromises()
    expect(w.get('[data-test=headline]').text()).toBe('All clear')
    expect(w.find('[data-test=repo-block]').exists()).toBe(false)
    expect(w.findAll('[data-test=project]')).toHaveLength(1)
  })

  it('focuses one project from the strip and clears the focus', async () => {
    const w = mountWith(mixedState())
    await flushPromises()
    await w.findAll('[data-test=segment]')[1].trigger('click')
    expect(w.find('[data-test=repo-block]').exists()).toBe(false)
    expect(w.find('[data-test=empty]').exists()).toBe(true)
    await w.get('[data-test=clear-all]').trigger('click')
    expect(w.findAll('[data-test=repo-block]')).toHaveLength(1)
  })

  it('narrows the list as you type and restores it when cleared', async () => {
    const w = mountWith(mixedState())
    await flushPromises()
    await w.get('[data-test=search]').setValue('zzz-no-match')
    expect(w.find('[data-test=repo-block]').exists()).toBe(false)
    expect(w.find('[data-test=no-projects]').exists()).toBe(true)
    await w.get('[data-test=clear-all]').trigger('click')
    expect(w.findAll('[data-test=repo-block]')).toHaveLength(1)
    expect(w.findAll('[data-test=project]')).toHaveLength(2)
  })

  it('applies a kind filter from the toolbar', async () => {
    const w = mountWith(mixedState())
    await flushPromises()
    await w.get('[data-test=kind-workflow]').trigger('click')
    expect(w.findAll('[data-test=row]')).toHaveLength(1)
    expect(w.get('[data-test=sev-all]').text()).toBe('All 1')
  })

  it('waits while the server has not finished its first refresh', async () => {
    const w = mountWith(state({ loaded: false, scan: { done: 4, total: 10 } }))
    await flushPromises()
    expect(w.find('[data-test=waiting]').exists()).toBe(true)
    expect(w.get('[data-test=loading-detail]').text()).toContain('4 of 10 scanned')
    expect(w.find('[data-test=headline]').exists()).toBe(false)
  })

  it('surfaces a connection error', async () => {
    const w = mountWith(new Error('offline'))
    await flushPromises()
    expect(w.get('[data-test=error]').text()).toContain('offline')
  })

  it('offers a rescan only when the server allows it', async () => {
    const on = mountWith(state({ manualRefresh: true, repos: [repo()] }))
    await flushPromises()
    expect(on.find('[data-test=rescan]').exists()).toBe(true)
    const off = mountWith(state({ manualRefresh: false, repos: [repo()] }))
    await flushPromises()
    expect(off.find('[data-test=rescan]').exists()).toBe(false)
  })

  it('refreshes one repo from its button, shows the new state and announces it', async () => {
    const first = state({
      manualRefresh: true,
      repos: [repo({ refreshedAt: '2026-10-02T10:00:00Z' })],
    })
    const after = { ...first, repo: 'o/r', repos: [repo({ refreshedAt: '2026-10-02T12:00:00Z' })] }
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        new Response(JSON.stringify(init?.method === 'POST' ? after : first), { status: 200 }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const w = mount(App, { global: { plugins: [createPinia()] } })
    await flushPromises()
    await w.get('[data-test=project] [data-test=repo-refresh]').trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledWith('/refresh?repo=o%2Fr', { method: 'POST' })
    expect(w.get('[data-test=announcement]').text()).toBe('Refreshed o/r')
  })

  it('shows the refusal message when a repo is refreshed too soon', async () => {
    const body = state({ manualRefresh: true, repos: [repo()] })
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? new Response('{}', { status: 429, headers: { 'Retry-After': '30' } })
            : new Response(JSON.stringify(body), { status: 200 }),
        ),
      ),
    )
    const w = mount(App, { global: { plugins: [createPinia()] } })
    await flushPromises()
    await w.get('[data-test=project] [data-test=repo-refresh]').trigger('click')
    await flushPromises()
    expect(w.get('[data-test=notice]').text()).toBe('o/r was refreshed recently. Try again in 30s')
    expect(
      w.get('[data-test=project] [data-test=repo-refresh]').attributes('disabled'),
    ).toBeUndefined()
  })

  it('hides the repo buttons when the server disables manual refresh', async () => {
    const w = mountWith(state({ manualRefresh: false, repos: [repo()] }))
    await flushPromises()
    expect(w.find('[data-test=repo-refresh]').exists()).toBe(false)
  })
})
