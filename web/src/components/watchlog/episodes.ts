import type { WatchRange } from '../../api'

// What someone has watched of a show is a list of episode ranges, in order
// and apart: [[1, 4], [6, 6]].

export const hasEpisode = (watched: readonly WatchRange[], n: number) => watched.some(([from, to]) => from <= n && n <= to)

export const countEpisodes = (watched: readonly WatchRange[]) => watched.reduce((sum, [from, to]) => sum + to - from + 1, 0)

// The first episode not watched: where to go on.
export const firstMissing = (watched: readonly WatchRange[]) => (watched.length === 0 || watched[0][0] > 1 ? 1 : watched[0][1] + 1)

// Every episode of a show of that length is watched: nothing left to watch
// it with anybody.
export const finished = (watched: readonly WatchRange[], episodes: number | null) => episodes !== null && firstMissing(watched) > episodes

export const lastEpisode = (watched: readonly WatchRange[]) => (watched.length === 0 ? 0 : watched[watched.length - 1][1])

// The episodes left out before the last one watched, as ranges.
export function skipped(watched: readonly WatchRange[]): WatchRange[] {
  const out: WatchRange[] = []
  let next = 1
  for (const [from, to] of watched) {
    if (from > next) out.push([next, from - 1])
    next = to + 1
  }
  return out
}

// "2–4" or "3".
export const rangeText = ([from, to]: WatchRange) => (from === to ? String(from) : `${from}–${to}`)

// watched with the episodes from..to added, or taken out.
export function withRange(watched: readonly WatchRange[], from: number, to: number, add: boolean): WatchRange[] {
  const out: WatchRange[] = []
  for (const [a, b] of watched) {
    if (b < from || a > to) out.push([a, b])
    else {
      if (a < from) out.push([a, from - 1])
      if (b > to) out.push([to + 1, b])
    }
  }
  if (!add) return out
  out.push([from, to])
  out.sort((x, y) => x[0] - y[0])
  // Ranges that touch are one.
  const joined: WatchRange[] = []
  for (const r of out) {
    const last = joined[joined.length - 1]
    if (last && r[0] <= last[1] + 1) last[1] = Math.max(last[1], r[1])
    else joined.push([r[0], r[1]])
  }
  return joined
}
