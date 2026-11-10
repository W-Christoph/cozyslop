import { useState } from 'preact/hooks'
import type { WatchAnime, WatchPerson, WatchRange } from '../../api'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Avatar } from '../ui/Avatar'
import { Cover } from './AnimeSearch'
import { EpisodeBar } from './EpisodeBar'
import { countEpisodes, hasEpisode, lastEpisode, rangeText, skipped } from './episodes'
import styles from './EpisodesDialog.module.css'

const MORE = 24 // episodes offered at a time while a show's length is unknown

// The episodes of one show for one person, each to tick and untick: by the
// person and by admins; everyone else sees them. Nothing in it moves when
// an episode is ticked: the buttons stay, switched off when they do nothing.
export function EpisodesDialog({ anime, person, self, watched, editable, ignored, onSet, onIgnore, onClose }: {
  anime: WatchAnime
  person: WatchPerson
  self: boolean // the person is me
  watched: readonly WatchRange[]
  editable: boolean
  ignored: boolean // not following the show any more: the others do not wait
  onSet: (from: number, to: number, watched: boolean) => void
  onIgnore: (ignored: boolean) => void
  onClose: () => void
}) {
  const last = lastEpisode(watched), count = countEpisodes(watched), gaps = skipped(watched)
  // A show still airing has no length: some episodes past the last one
  // watched when this opened, and more on request.
  const [shown, setShown] = useState(() => anime.episodes ?? Math.ceil((last + 1) / MORE) * MORE)
  const total = Math.max(shown, last)
  return (
    <Modal title={self ? 'Your episodes' : `${person.nickname}’s episodes`} size="md" onClose={onClose}>
      <div class={styles.head}>
        <Cover anime={anime} size={64} />
        <div class={styles.about}>
          <strong class={styles.show}>{anime.title}</strong>
          <span class={styles.person}><Avatar src={person.avatarUrl} size={18} /><span style={{ color: person.nameColor }}>{person.nickname}</span></span>
        </div>
        <div class={styles.count}>
          <strong>{count}</strong>{anime.episodes !== null && <span> / {anime.episodes}</span>}
          <small>watched</small>
        </div>
      </div>
      <EpisodeBar class={styles.bar} watched={watched} episodes={anime.episodes} />
      <p class={`${styles.skipped} ${gaps.length > 0 ? styles.gaps : ''}`}>{gaps.length > 0 ? `Skipped ${gaps.map(rangeText).join(', ')}` : count > 0 ? 'Nothing skipped' : '\u00a0'}</p>
      <div class={styles.toolbar}>
        <span class={styles.label}>Episodes</span>
        {editable && <>
          <Button size="sm" variant="ghost" disabled={gaps.length === 0} title="Tick every episode before the last one watched" onClick={() => onSet(1, last, true)}>Fill gaps</Button>
          {anime.episodes !== null && <Button size="sm" variant="ghost" disabled={count >= anime.episodes} onClick={() => onSet(1, anime.episodes!, true)}>Tick all</Button>}
          <Button size="sm" variant="ghost" disabled={count === 0} onClick={() => onSet(1, 9999, false)}>Untick all</Button>
        </>}
      </div>
      <div class={styles.grid} role="group" aria-label="Episodes">
        {Array.from({ length: total }, (_, i) => i + 1).map((n) => {
          const on = hasEpisode(watched, n)
          return <button key={n} type="button" class={styles.episode} aria-pressed={on} disabled={!editable}
            aria-label={`Episode ${n}${on ? ', watched' : ''}`} onClick={() => onSet(n, n, !on)}>{n}</button>
        })}
      </div>
      <div class={styles.foot}>
        <span>{editable ? 'Click an episode to tick or untick it.' : `Only ${person.nickname} and admins can change this.`}</span>
        {anime.episodes === null && <Button size="sm" variant="ghost" onClick={() => setShown(total + MORE)}>More episodes</Button>}
      </div>
      {(editable || ignored) && (
        <div class={styles.following}>
          <div class={styles.followingText}>
            <strong>{ignored ? 'Not following any more' : 'Following this show'}</strong>
            <span>{ignored ? `The others do not wait for ${self ? 'you' : person.nickname}. What was watched stays.` : `The others wait for ${self ? 'you' : person.nickname} before they watch on.`}</span>
          </div>
          {editable && <Button size="sm" variant="secondary" onClick={() => onIgnore(!ignored)}>{ignored ? 'Follow again' : 'Stop following'}</Button>}
        </div>
      )}
    </Modal>
  )
}
