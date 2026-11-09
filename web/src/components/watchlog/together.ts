import type { WatchAnime } from '../../api'

export interface Together {
  next: number | null // the episode everyone can watch next; null when all have finished
  started: boolean // someone has watched some of it
  behind: { username: string; by: number }[] // who is behind the furthest, and by how much
}

// What a group can watch together: the episode after the one the least far
// of them has watched. episodeOf is 0 for someone who has not started.
export function together(anime: WatchAnime, usernames: readonly string[], episodeOf: (username: string) => number): Together {
  const watched = usernames.map((u) => ({ username: u, episode: episodeOf(u) }))
  const low = Math.min(...watched.map((w) => w.episode)), high = Math.max(0, ...watched.map((w) => w.episode))
  const next = low + 1
  return {
    next: watched.length === 0 || (anime.episodes !== null && next > anime.episodes) ? null : next,
    started: high > 0,
    behind: watched.filter((w) => w.episode < high).map((w) => ({ username: w.username, by: high - w.episode })),
  }
}
