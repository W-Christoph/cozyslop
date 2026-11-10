import type { WatchRange } from '../../api'
import { lastEpisode } from './episodes'
import styles from './EpisodeBar.module.css'

// The episodes of a show as a bar, filled where they are watched: episodes
// left out show as holes. Without a known length it ends at the last one
// watched.
export function EpisodeBar({ watched, episodes, class: className }: { watched: readonly WatchRange[]; episodes: number | null; class?: string }) {
  const total = Math.max(episodes ?? 0, lastEpisode(watched), 1)
  const done = episodes !== null && watched.length === 1 && watched[0][0] === 1 && watched[0][1] >= episodes
  return (
    <span class={`${styles.bar} ${done ? styles.done : ''} ${className ?? ''}`} aria-hidden="true">
      {watched.map(([from, to]) => (
        <span key={from} class={styles.part} style={{ left: `${((from - 1) / total) * 100}%`, width: `${((to - from + 1) / total) * 100}%` }} />
      ))}
    </span>
  )
}
