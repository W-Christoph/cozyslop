import type { WatchAnime, WatchProgress, WatchRange } from '../../api'
import { countEpisodes, finished, firstMissing, hasEpisode } from './episodes'

export interface Together {
  next: number | null // the first episode not everyone has watched; null when all have finished
  started: boolean // someone has watched some of it
  behind: { username: string; by: number }[] // who has to go on at an earlier episode than the one furthest on, and how many earlier
}

// What a group can watch together: the first episode one of them has not
// watched yet. watchedOf is empty for someone who has not started.
export function together(anime: WatchAnime, usernames: readonly string[], watchedOf: (username: string) => readonly WatchRange[]): Together {
  const people = usernames.map((u) => ({ username: u, count: countEpisodes(watchedOf(u)), next: firstMissing(watchedOf(u)) }))
  // Behind is counted in episodes seen without a gap: one left out holds
  // the group up as much as one never reached.
  const next = Math.min(...people.map((p) => p.next)), high = Math.max(1, ...people.map((p) => p.next))
  return {
    next: people.length === 0 || (anime.episodes !== null && next > anime.episodes) ? null : next,
    started: people.some((p) => p.count > 0),
    behind: people.filter((p) => p.next < high).map((p) => ({ username: p.username, by: high - p.next })),
  }
}

// What the people here can watch on of a show that group follows: the first
// episode one of those here has not seen. Someone of the group who is not
// here is waited for only if they have not seen that episode: watching it
// without them would leave them behind. null if nobody here follows the
// show, or those here have finished it.
export function watchNext(anime: WatchAnime, group: readonly string[], here: ReadonlySet<string>, watchedOf: (username: string) => readonly WatchRange[]) {
  const t = together(anime, group.filter((u) => here.has(u)), watchedOf)
  if (t.next === null) return null
  const next = t.next
  return { next, behind: t.behind, waiting: group.filter((u) => !here.has(u) && !hasEpisode(watchedOf(u), next)) }
}

// ready: those here who follow it are at the same episode. catchup: one of them is behind.
// waiting: someone who follows it is not here and has not seen the next
// episode. solo: only one of several here watches it. new: on the list, and
// nobody here has started it.
export type WatchState = 'ready' | 'catchup' | 'waiting' | 'solo' | 'new'
export const watchStates: readonly WatchState[] = ['ready', 'catchup', 'waiting', 'solo', 'new']

export interface WatchOption {
  anime: WatchAnime
  state: WatchState
  next: number
  group: string[] // who is in the middle of it, here or not
  waiting: string[] // of them, who is not here and has not seen the next episode
  behind: { username: string; by: number }[]
  unstarted: string[] // who is here and has not started it
  needs: string[] // who is here and would see the next episode for the first time
  recent: number // when it was last watched; for a new show, when it was added
}

// What the people here can watch, best first. A show's group is everyone
// who has watched some of it, has not finished it and has not stopped
// following it. Shows nobody here has started come last, newest first.
export function watchOptions(anime: readonly WatchAnime[], here: readonly string[], progress: readonly WatchProgress[],
  ignoredOf: (animeId: number, username: string) => boolean): WatchOption[] {
  const present = new Set(here)
  const options: WatchOption[] = []
  for (const a of anime) {
    const rows = progress.filter((p) => p.animeId === a.id && countEpisodes(p.watched) > 0)
    const watched = new Map(rows.map((p) => [p.username, p.watched]))
    const of = (u: string) => watched.get(u) ?? []
    const following = rows.filter((p) => !finished(p.watched, a.episodes) && !ignoredOf(a.id, p.username))
    const group = following.map((p) => p.username)
    const unstarted = here.filter((u) => !watched.has(u) && !ignoredOf(a.id, u))
    const t = watchNext(a, group, present, of)
    if (t) {
      const alone = here.length > 1 && group.filter((u) => present.has(u)).length < 2
      options.push({
        anime: a, state: t.waiting.length ? 'waiting' : alone ? 'solo' : t.behind.length ? 'catchup' : 'ready', next: t.next, group,
        waiting: t.waiting, behind: t.behind, unstarted,
        needs: group.filter((u) => present.has(u) && !hasEpisode(of(u), t.next)),
        recent: Math.max(...following.map((p) => p.watchedAt)),
      })
    } else if (unstarted.length > 0 && !here.some((u) => watched.has(u))) {
      options.push({ anime: a, state: 'new', next: 1, group, waiting: [], behind: [], unstarted, needs: unstarted, recent: a.addedAt ?? 0 })
    }
  }
  // In a section, the fewer of those here a show is new to, the earlier it comes.
  return options.sort((a, b) => watchStates.indexOf(a.state) - watchStates.indexOf(b.state)
    || (a.state === 'new' ? 0 : a.unstarted.length - b.unstarted.length)
    || b.recent - a.recent || a.anime.title.localeCompare(b.anime.title))
}
