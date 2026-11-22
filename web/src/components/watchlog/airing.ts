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

// The page of episode n where the show is watched; "" if nobody knows.
export function episodeAddress(anime: WatchAnime, n: number) {
  return anime.watchUrl ? anime.watchUrl.replaceAll('{ep}', String(n)) : ''
}
