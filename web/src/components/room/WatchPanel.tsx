import { useEffect, useState } from 'preact/hooks'
import type { WatchProposal, WatchViewer } from '../../room/protocol'
import { AnimeSearch, Cover } from '../watchlog/AnimeSearch'
import { Icon } from '../ui/Icon'
import { useRoomStore, watchlogWanted } from './RoomContext'
import styles from './WatchPanel.module.css'

// The episode VLC or Firefox plays, as a bar at the top of the chat: the
// show's whole name, on two lines if it needs them. Accounts get to the
// watchlog from it, and say which show it is where that was not recognised
// ("Link") or is AniList's guess, which may be wrong (a small button): the
// name it goes by is that show's from then on, and the button is gone.
function WatchPlayingBar() {
  const store = useRoomStore()
  const playing = store.watchPlaying.value
  const [linking, setLinking] = useState(false)
  if (!playing) return null
  const known = playing.animeId !== 0
  const line = <>
    <Icon name="play" size={14} />
    <span class={styles.playingText}>
      <span class={styles.label}>Watching{!known && ' (not recognised)'}</span>
      <strong class={styles.show} title={playing.title}>{playing.title}</strong>
    </span>
    <span class={styles.ep}>Ep {playing.episode}</span>
  </>
  if (store.self.value?.anonymous !== false) return <div role="status"><div class={styles.playing}>{line}</div></div>
  return (
    <div role="status">
      <div class={styles.bar}>
        <button type="button" class={`${styles.playing} ${styles.open}`} title="Open the watchlog" onClick={() => { watchlogWanted.value = true }}>{line}</button>
        {!playing.linked && (
          <button type="button" class={`${styles.linkShow} ${known ? '' : styles.wanted}`} aria-expanded={linking}
            aria-label={known ? 'Wrong show? Link another' : 'Link a show'}
            title={known ? 'Not this show? Link it to another' : 'Say which show this is. Its other episodes are then recognised.'}
            onClick={() => setLinking(!linking)}><Icon name="link" size={14} />{!known && 'Link'}</button>
        )}
      </div>
      {linking && !playing.linked && (
        <div class={styles.linking}>
          <AnimeSearch autoFocus placeholder="Search AniList…" onPick={(anime) => { store.linkWatch(anime.id); setLinking(false) }} />
        </div>
      )}
    </div>
  )
}

// The watchlog at the top of the chat: the bar of what plays and, once an
// episode ended, under it and of a piece with it the suggestion to mark it
// watched. Whoever saw it is asked about themselves, once, and their answer
// closes it for them alone; an admin gets everyone to tick, and settles it
// for the room. The bar is left out over the stream in fullscreen, where it
// would cover the picture for the whole episode.
export function WatchPanel({ bar }: { bar: boolean }) {
  const store = useRoomStore()
  const proposal = store.watchProposal.value
  const self = store.self.value
  const mine = proposal?.viewers.find((v) => v.key === self?.key)
  // Keyed: a new suggestion starts without the last one's show search open.
  return <>
    {bar && <WatchPlayingBar />}
    {proposal && self?.anonymous === false && (store.rights.value.admin
      ? <WatchCard key={proposal.id} proposal={proposal} />
      : mine && !mine.answered && <WatchAsk key={proposal.id} proposal={proposal} viewer={mine} />)}
  </>
}

// "Did you watch it?", for one who saw some of it. Unanswered, it is saved
// as suggested: marked for who saw most of the episode.
function WatchAsk({ proposal: p, viewer }: { proposal: WatchProposal; viewer: WatchViewer }) {
  const store = useRoomStore()
  const [fixing, setFixing] = useState(false)
  const left = useCountdown(p.applyAt)
  const max = p.episodes ?? 9999
  const known = p.animeId !== 0
  return (
    <section class={styles.card} aria-label="Mark episode watched">
      <div class={styles.head}>
        {known && <Cover anime={p} size={48} />}
        <div class={styles.headText}>
          <span class={styles.label}>Watched?</span>
          <strong class={styles.title} title={p.file}>{known ? p.title : 'Which show was this?'}</strong>
          <div class={styles.episode}>
            Episode
            {fixing && <button type="button" class={styles.step} aria-label="Episode before" disabled={p.episode <= 1}
              onClick={() => store.setWatchEpisode(p.id, p.episode - 1)}>−</button>}
            <span class={styles.number}>{p.episode}</span>
            {fixing && <button type="button" class={styles.step} aria-label="Episode after" disabled={p.episode >= max}
              onClick={() => store.setWatchEpisode(p.id, p.episode + 1)}>+</button>}
            <span>· you saw {Math.round(viewer.coverage * 100)}%</span>
          </div>
        </div>
      </div>
      {!known && <p class={styles.file} title={p.file}>{p.file}</p>}
      {fixing || !known
        ? <AnimeSearch autoFocus={fixing} placeholder="Search AniList…" onPick={(anime) => { store.setWatchAnime(p.id, anime.id); setFixing(false) }} />
        : <button type="button" class={styles.link} onClick={() => setFixing(true)}>Wrong show or episode?</button>}
      <div class={styles.foot}>
        <span class={styles.muted}>
          {!known ? 'Saved once the show is picked.' : !p.applyAt ? `The show has ${p.episodes} episodes.`
            : viewer.checked ? `Marked for you in ${left}` : 'Not marked unless you say yes'}
        </span>
        <span class={styles.answers}>
          <button type="button" class={styles.no} onClick={() => store.answerWatch(p.id, false)}>No</button>
          <button type="button" class={styles.save} onClick={() => store.answerWatch(p.id, true)}>Yes, mark it</button>
        </span>
      </div>
    </section>
  )
}

// The suggestion for an admin: who saw it, to tick, and saving or dropping
// it for everyone.
function WatchCard({ proposal: p }: { proposal: WatchProposal }) {
  const store = useRoomStore()
  const [picking, setPicking] = useState(false)
  const left = useCountdown(p.applyAt)
  const max = p.episodes ?? 9999
  const fits = p.episode <= max // a file numbered past the show's end waits to be fixed
  return (
    <section class={styles.card} aria-label="Mark episode watched">
      <div class={styles.head}>
        {p.animeId !== 0 && <Cover anime={p} size={48} />}
        <div class={styles.headText}>
          <span class={styles.label}>Watched</span>
          <strong class={styles.title} title={p.file}>{p.animeId === 0 ? 'Which show was this?' : p.title}</strong>
          <div class={styles.episode}>
            Episode
            <button type="button" class={styles.step} aria-label="Episode before" disabled={p.episode <= 1}
              onClick={() => store.setWatchEpisode(p.id, p.episode - 1)}>−</button>
            <span class={styles.number}>{p.episode}</span>
            <button type="button" class={styles.step} aria-label="Episode after" disabled={p.episode >= max}
              onClick={() => store.setWatchEpisode(p.id, p.episode + 1)}>+</button>
          </div>
        </div>
        <button type="button" class={styles.close} aria-label="Dismiss" title="Don’t save, for everyone"
          onClick={() => store.settleWatch(p.id, false)}><Icon name="x" size={16} /></button>
      </div>
      {p.animeId === 0 && <p class={styles.file} title={p.file}>{p.file}</p>}
      {picking || p.animeId === 0
        ? <AnimeSearch autoFocus={picking} placeholder="Search AniList…" onPick={(anime) => { store.setWatchAnime(p.id, anime.id); setPicking(false) }} />
        : <button type="button" class={styles.link} onClick={() => setPicking(true)}>Wrong show?</button>}
      <ul class={styles.viewers} aria-label="Who saw it">
        {p.viewers.map((v) => (
          <li key={v.key}>
            <label class={styles.viewer}>
              <input type="checkbox" checked={v.checked}
                onChange={(e) => store.checkWatcher(p.id, v.key, e.currentTarget.checked)} />
              <span class={styles.name}>{v.nickname}</span>
              <span class={styles.muted}>{v.answered ? (v.checked ? 'said yes · ' : 'said no · ') : ''}{Math.round(v.coverage * 100)}%</span>
            </label>
          </li>
        ))}
      </ul>
      <div class={styles.foot}>
        <span class={styles.muted}>{p.applyAt ? `Saves in ${left}` : p.animeId === 0 ? 'Pick the show to save.' : `The show has ${p.episodes} episodes.`}</span>
        <button type="button" class={styles.save} disabled={p.animeId === 0 || !fits}
          onClick={() => store.settleWatch(p.id, true)}>Save</button>
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
