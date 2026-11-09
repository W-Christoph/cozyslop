import { useEffect, useState } from 'preact/hooks'
import type { WatchProposal } from '../../room/protocol'
import { AnimeSearch, Cover } from '../watchlog/AnimeSearch'
import { Icon } from '../ui/Icon'
import { useRoomStore } from './RoomContext'
import styles from './WatchPanel.module.css'

// The watchlog in the room, above the chat input: the episode VLC plays, and
// once it ended the suggestion to mark it watched for those who saw it.
export function WatchPanel() {
  const store = useRoomStore()
  const proposal = store.watchProposal.value
  const playing = store.watchPlaying.value
  // Keyed: a new suggestion starts without the last one's show search open.
  if (proposal) return <WatchCard key={proposal.id} proposal={proposal} />
  if (!playing) return null
  return (
    <div class={styles.playing} role="status">
      <Icon name="play" size={12} />
      <span class={styles.playingText}>
        Watching <strong>{playing.title}</strong> · Ep {playing.episode}
        {playing.animeId === 0 && <span class={styles.muted}> (not recognised)</span>}
      </span>
    </div>
  )
}

function WatchCard({ proposal: p }: { proposal: WatchProposal }) {
  const store = useRoomStore()
  // Only accounts may change it; others see it.
  const canEdit = store.self.value?.anonymous === false
  const [picking, setPicking] = useState(false)
  const left = useCountdown(p.applyAt)
  const max = p.episodes ?? 9999
  const fits = p.episode <= max // a file numbered past the show's end waits to be fixed
  return (
    <section class={styles.card} aria-label="Mark episode watched">
      <div class={styles.head}>
        {p.animeId !== 0 && <Cover anime={p} size={48} />}
        <div class={styles.headText}>
          <strong class={styles.title} title={p.file}>{p.animeId === 0 ? 'Which show was this?' : p.title}</strong>
          <div class={styles.episode}>
            Episode
            {canEdit && <button type="button" class={styles.step} aria-label="Episode before" disabled={p.episode <= 1}
              onClick={() => store.setWatchEpisode(p.id, p.episode - 1)}>−</button>}
            <span class={styles.number}>{p.episode}</span>
            {canEdit && <button type="button" class={styles.step} aria-label="Episode after" disabled={p.episode >= max}
              onClick={() => store.setWatchEpisode(p.id, p.episode + 1)}>+</button>}
          </div>
        </div>
        {canEdit && <button type="button" class={styles.close} aria-label="Dismiss" title="Don’t save"
          onClick={() => store.settleWatch(p.id, false)}><Icon name="x" size={16} /></button>}
      </div>
      {p.animeId === 0 && <p class={styles.file} title={p.file}>{p.file}</p>}
      {canEdit && (picking || p.animeId === 0
        ? <AnimeSearch autoFocus={picking} placeholder="Search AniList…" onPick={(anime) => { store.setWatchAnime(p.id, anime.id); setPicking(false) }} />
        : <button type="button" class={styles.link} onClick={() => setPicking(true)}>Wrong show?</button>)}
      <ul class={styles.viewers} aria-label="Who saw it">
        {p.viewers.map((v) => (
          <li key={v.key}>
            <label class={styles.viewer}>
              <input type="checkbox" checked={v.checked} disabled={!canEdit}
                onChange={(e) => store.checkWatcher(p.id, v.key, e.currentTarget.checked)} />
              <span class={styles.name}>{v.nickname}</span>
              <span class={styles.muted}>{Math.round(v.coverage * 100)}%</span>
            </label>
          </li>
        ))}
      </ul>
      <div class={styles.foot}>
        <span class={styles.muted}>{p.applyAt ? `Saves in ${left}` : p.animeId === 0 ? 'Pick the show to save.' : `The show has ${p.episodes} episodes.`}</span>
        {canEdit && <button type="button" class={styles.save} disabled={p.animeId === 0 || !fits}
          onClick={() => store.settleWatch(p.id, true)}>Save</button>}
      </div>
    </section>
  )
}

// "1:42" until time (unix ms), counting down every second.
function useCountdown(time: number) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (!time) return
    setNow(Date.now())
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [time])
  const seconds = Math.max(0, Math.ceil((time - now) / 1000))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}
