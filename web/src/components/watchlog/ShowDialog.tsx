import { useRef } from 'preact/hooks'
import type { WatchAnime, WatchPerson, WatchRange } from '../../api'
import { Button, ButtonLink } from '../Button'
import { Modal } from '../Modal'
import { Avatar } from '../ui/Avatar'
import { Badge } from '../ui/Badge'
import { Cover } from './AnimeSearch'
import { EpisodeBar } from './EpisodeBar'
import { countEpisodes, finished, firstMissing } from './episodes'
import styles from './ShowDialog.module.css'

// One show with everything to do about it: who has watched how far, with
// the next episode to tick and dropping it for each person one may change;
// and the show itself, to bring up to date from AniList or take off the list.
export function ShowDialog({ anime, people, self, playing, busy, watchedOf, ignoredOf, mayChange, mayRemove, onEpisodes, onSet, onIgnore, onRefresh, onRemove, onClose }: {
  anime: WatchAnime
  people: WatchPerson[] // me, those selected and everyone who has watched some of it
  self: string // my username
  playing: boolean // the room is watching it
  busy: boolean // AniList is being asked
  watchedOf: (username: string) => readonly WatchRange[]
  ignoredOf: (username: string) => boolean
  mayChange: (username: string) => boolean
  mayRemove: boolean
  onEpisodes: (person: WatchPerson) => void
  onSet: (username: string, episode: number) => void // ticks one episode
  onIgnore: (username: string, ignored: boolean) => void
  onRefresh: () => void
  onRemove: () => void
  onClose: () => void
}) {
  // A second click right after the first is not meant for the next episode.
  const last = useRef(0)
  const tick = (username: string, episode: number) => {
    if (Date.now() - last.current < 600) return
    last.current = Date.now()
    onSet(username, episode)
  }
  return (
    <Modal title={anime.title} size="md" onClose={onClose}>
      <div class={styles.head}>
        <Cover anime={anime} size={96} />
        <div class={styles.about}>
          {anime.titleEnglish && anime.titleEnglish.toLowerCase() !== anime.title.toLowerCase() && <strong class={styles.title}>{anime.titleEnglish}</strong>}
          <span class={styles.length}>
            {anime.episodes !== null ? `${anime.episodes} episode${anime.episodes === 1 ? '' : 's'}` : 'Length not known yet (still airing)'}
            {playing && <Badge tone="accent" icon="play">Watching now</Badge>}
          </span>
        </div>
      </div>
      <h3 class={styles.label}>Who has watched what</h3>
      <ul class={styles.people}>
        {people.map((p) => {
          const watched = watchedOf(p.username), count = countEpisodes(watched), next = firstMissing(watched)
          const done = finished(watched, anime.episodes), dropped = ignoredOf(p.username), editable = mayChange(p.username)
          const me = p.username === self
          return (
            <li key={p.username} class={`${styles.person} ${dropped ? styles.dropped : ''}`}>
              <Avatar src={p.avatarUrl} size={28} />
              <div class={styles.progress}>
                <span class={styles.name}>
                  <span style={{ color: p.nameColor }}>{me ? 'You' : p.nickname}</span>
                  {dropped ? <Badge title={`The others do not wait for ${me ? 'you' : p.nickname}`}>{count === 0 ? 'Not interested' : 'Dropped'}</Badge> : done ? <Badge tone="success" icon="check">Finished</Badge> : null}
                  {/* At the line's end, so that its changing label moves nothing. */}
                  {editable && !done && (
                    <button type="button" class={styles.drop} title={dropped ? 'The others wait again before they watch on' : 'The others stop waiting. What was watched stays.'}
                      onClick={() => onIgnore(p.username, !dropped)}>{dropped ? 'Pick up again' : count === 0 ? 'Not interested' : 'Drop'}</button>
                  )}
                </span>
                <EpisodeBar watched={watched} episodes={anime.episodes} />
                <span class={styles.count}>{count === 0 ? 'Not started' : `${count === next - 1 ? '' : `Up to ep ${next - 1} · `}${count}${anime.episodes !== null ? ` / ${anime.episodes}` : ''} watched`}</span>
              </div>
              <div class={styles.actions}>
                {editable && !done && <Button size="sm" icon="check" class={styles.watched} title={`Mark episode ${next} as watched for ${me ? 'you' : p.nickname}`} aria-label={`Mark ep ${next}: as watched for ${me ? 'you' : p.nickname}`} onClick={() => tick(p.username, next)}>Mark ep {next}</Button>}
                <Button size="sm" variant="ghost" title="Tick and untick single episodes" onClick={() => onEpisodes(p)}>{editable ? 'Episodes…' : 'Episodes'}</Button>
              </div>
            </li>
          )
        })}
      </ul>
      <h3 class={styles.label}>Manage the show</h3>
      <div class={styles.manage}>
        <ButtonLink size="sm" icon="link" href={`https://anilist.co/anime/${anime.id}`} target="_blank" rel="noreferrer noopener">Open on AniList</ButtonLink>
        <Button size="sm" icon="refresh" disabled={busy} title="Fetch the title, cover and number of episodes again" onClick={onRefresh}>{busy ? 'Updating…' : 'Update from AniList'}</Button>
        {mayRemove && <Button size="sm" variant="danger-outline" icon="trash" class={styles.remove} onClick={onRemove}>Remove from watchlog</Button>}
      </div>
      {!mayRemove && <p class={styles.note}>Only admins can remove a show someone has watched.</p>}
    </Modal>
  )
}
