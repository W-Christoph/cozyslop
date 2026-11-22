import type { WatchAnime } from '../../api'

// Whether episode n of a show is out, as far as AniList says: those before
// the next one announced are, that one once its time has come, and none
// after it.
export function aired(anime: WatchAnime, n: number, now = Date.now()) {
  if (anime.nextEpisode == null || anime.nextAiringAt == null || n < anime.nextEpisode) return true
  return n === anime.nextEpisode && anime.nextAiringAt * 1000 <= now
}

// "in 2d 5h", "in 3h 20m", "in 12m": how long until at (unix seconds).
export function airsIn(at: number, now = Date.now()) {
  const minutes = Math.max(1, Math.ceil((at * 1000 - now) / 60000))
  const days = Math.floor(minutes / 1440), hours = Math.floor((minutes % 1440) / 60)
  if (days > 0) return `in ${days}d${hours ? ` ${hours}h` : ''}`
  return hours > 0 ? `in ${hours}h${minutes % 60 ? ` ${minutes % 60}m` : ''}` : `in ${minutes}m`
}

// "Sat 14:00", with the date as well beyond the coming week.
export function airsOn(at: number, now = Date.now()) {
  const date = new Date(at * 1000)
  const far = at * 1000 - now > 6 * 86400000
  return date.toLocaleString(undefined, far
    ? { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }
    : { weekday: 'short', hour: '2-digit', minute: '2-digit' })
}

// The page of episode n where the show is watched; "" if nobody knows. A
// site whose addresses do not say the episode has only the latest one's.
export function episodeAddress(anime: WatchAnime, n: number) {
  // "{ep+1}" where the site counts one more than the show.
  if (anime.watchUrl) return anime.watchUrl.replace(/\{ep(?:([+-])(\d{1,4}))?\}/g, (_, sign: string | undefined, d: string | undefined) =>
    String(n + (sign === '-' ? -1 : 1) * Number(d ?? 0)))
  return anime.pageUrl && anime.pageEpisode === n ? anime.pageUrl : ''
}

// The latest episode with a page, for a show with nothing new to watch:
// the one that played last, or else the last one out; null if none is known.
export function latestPage(anime: WatchAnime, now = Date.now()): { episode: number; address: string } | null {
  if (anime.pageUrl && anime.pageEpisode) return { episode: anime.pageEpisode, address: anime.pageUrl }
  const out = anime.nextEpisode != null ? (aired(anime, anime.nextEpisode, now) ? anime.nextEpisode : anime.nextEpisode - 1) : anime.episodes
  return anime.watchUrl && out != null && out >= 1 ? { episode: out, address: episodeAddress(anime, out) } : null
}
