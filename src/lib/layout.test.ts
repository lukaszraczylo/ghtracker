import { describe, expect, it } from 'vitest'
import { balanceColumns } from './layout'

const weight = (n: number) => n

describe('balanceColumns', () => {
  it('returns one column holding everything in order', () => {
    expect(balanceColumns([3, 1, 2], 1, weight)).toEqual([[3, 1, 2]])
  })

  it('puts each item in the currently shortest column', () => {
    // heights after each step: [5,0] -> [5,2] -> [5,5] -> [5,6]... ties go to the first column
    expect(balanceColumns([5, 2, 3, 1], 2, weight)).toEqual([
      [5, 1],
      [2, 3],
    ])
  })

  it('keeps the heaviest-first reading order within each column', () => {
    const [left, right] = balanceColumns([9, 8, 7, 6, 5], 2, weight)
    expect(left).toEqual([9, 6, 5])
    expect(right).toEqual([8, 7])
  })

  it('handles empty input and clamps invalid column counts', () => {
    expect(balanceColumns([], 2, weight)).toEqual([[], []])
    expect(balanceColumns([1, 2], 0, weight)).toEqual([[1, 2]])
    expect(balanceColumns([1, 2], 2.9, weight)).toHaveLength(2)
  })
})
