import type { WatchAnime, WatchRange } from '../../api'
import { countEpisodes, firstMissing } from './episodes'

export interface Together {
  next: number | null // the first episode not everyone has watched; null when all have finished
  started: boolean // someone has watched some of it
  behind: { username: string; by: number }[] // who has watched fewer episodes than the one with the most, and how many
}

// What a group can watch together: the first episode one of them has not
// watched yet. watchedOf is empty for someone who has not started.
export function together(anime: WatchAnime, usernames: readonly string[], watchedOf: (username: string) => readonly WatchRange[]): Together {
  const people = usernames.map((u) => ({ username: u, count: countEpisodes(watchedOf(u)), next: firstMissing(watchedOf(u)) }))
  const next = Math.min(...people.map((p) => p.next)), high = Math.max(0, ...people.map((p) => p.count))
  return {
    next: people.length === 0 || (anime.episodes !== null && next > anime.episodes) ? null : next,
    started: high > 0,
    behind: people.filter((p) => p.count < high).map((p) => ({ username: p.username, by: high - p.count })),
  }
}
