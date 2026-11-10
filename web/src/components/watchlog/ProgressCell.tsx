import type { WatchRange } from '../../api'
import { EpisodeBar } from './EpisodeBar'
import { countEpisodes, rangeText, skipped } from './episodes'
import styles from './ProgressCell.module.css'

// One person's progress in one show: how many episodes they have watched,
// over a bar of which. Opens the episodes themselves. Its size is fixed, so
// that the table keeps still when a number changes.
export function ProgressCell({ watched, episodes, ignored, label, onOpen }: {
  watched: readonly WatchRange[]
  episodes: number | null
  ignored: boolean // not following the show any more
  label: string // for screen readers: whose progress in what
  onOpen: () => void
}) {
  const count = countEpisodes(watched)
  const gaps = skipped(watched).map(rangeText).join(', ')
  const done = episodes !== null && count >= episodes
  return (
    <button type="button" class={`${styles.cell} ${ignored ? styles.ignored : ''}`} onClick={onOpen}
      title={[ignored ? 'Not following any more' : '', gaps ? `Skipped ${gaps}` : ''].filter(Boolean).join(' · ') || undefined}
      aria-label={`${label}: ${count === 0 ? 'not started' : `${count} watched`}${gaps ? `, skipped ${gaps}` : ''}${ignored ? ', not following any more' : ''}. Show episodes`}>
      <span class={`${styles.value} ${done ? styles.done : ''} ${count === 0 ? styles.none : ''}`}>
        {count === 0 ? '—' : episodes ? <>{count}<span class={styles.of}> / {episodes}</span></> : count}
      </span>
      <EpisodeBar class={`${styles.bar} ${count === 0 ? styles.empty : ''}`} watched={watched} episodes={episodes} />
    </button>
  )
}
