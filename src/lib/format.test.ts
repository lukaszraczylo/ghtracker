import { describe, expect, it } from 'vitest'
import { ageLabel, ageSince, isZeroTime, secondsSince, shorten } from './format'

describe('ageLabel', () => {
  it.each([
    [-60, '0m'],
    [300, '5m'],
    [5400, '1h'],
    [47 * 3600, '47h'],
    [49 * 3600, '2d'],
    [10 * 86400, '10d'],
  ])('%is -> %s', (seconds, want) => {
    expect(ageLabel(seconds)).toBe(want)
  })
})

describe('time helpers', () => {
  const now = Date.parse('2026-10-02T12:00:00Z')

  it('measures seconds since a timestamp', () => {
    expect(secondsSince('2026-10-02T11:00:00Z', now)).toBe(3600)
  })

  it('returns an empty age for unparsable input', () => {
    expect(ageSince('not a date', now)).toBe('')
    expect(ageSince('2026-10-02T09:00:00Z', now)).toBe('3h')
  })

  it('treats the Go zero time as unknown', () => {
    expect(isZeroTime('0001-01-01T00:00:00Z')).toBe(true)
    expect(isZeroTime('')).toBe(true)
    expect(isZeroTime('2026-10-02T12:00:00Z')).toBe(false)
  })
})

describe('shorten', () => {
  it('keeps short text and cuts long text by characters, not bytes', () => {
    expect(shorten('ok')).toBe('ok')
    const long = 'é'.repeat(200)
    const out = shorten(long)
    expect(Array.from(out)).toHaveLength(90)
    expect(out.endsWith('…')).toBe(true)
  })
})
