/**
 * Splits items into columns so the columns end at about the same height. Each item goes to the
 * currently shortest column, which keeps the original order readable (worst items first).
 */
export function balanceColumns<T>(items: T[], columns: number, weight: (item: T) => number): T[][] {
  const count = Math.max(1, Math.floor(columns))
  const out: T[][] = Array.from({ length: count }, () => [])
  const heights = new Array<number>(count).fill(0)
  for (const item of items) {
    let target = 0
    for (let i = 1; i < count; i++) if (heights[i] < heights[target]) target = i
    out[target].push(item)
    heights[target] += weight(item)
  }
  return out
}
