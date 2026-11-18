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
// episode ended, under it and of a piece with it the question whether you
// watched it. Everyone who saw some of it is asked about themselves alone,
// and sees only how much they saw; their answer closes it for them. The bar
// is left out over the stream in fullscreen, where it would cover the
// picture for the whole episode.
export function WatchPanel({ bar }: { bar: boolean }) {
  const store = useRoomStore()
  const proposal = store.watchProposal.value
  const self = store.self.value
  const mine = proposal?.viewers.find((v) => v.key === self?.key)
  // Keyed: a new suggestion starts without the last one's show search open.
  return <>
    {bar && <WatchPlayingBar />}
    {proposal && mine && !mine.answered && <WatchAsk key={proposal.id} proposal={proposal} viewer={mine} />}
  </>
}

// "Did you watch it?", for one who saw some of it. Unanswered, it is saved
// as suggested: marked if they saw most of the episode (60%), not if less.
// The suggested answer is the highlighted button.
function WatchAsk({ proposal: p, viewer }: { proposal: WatchProposal; viewer: WatchViewer }) {
  const store = useRoomStore()
  const [fixing, setFixing] = useState(false)
  const left = useCountdown(p.applyAt)
  const max = p.episodes ?? 9999
  const known = p.animeId !== 0
  // Yes needs a show, and an episode it has.
  const saveable = known && p.episode <= max
  const yes = viewer.checked && saveable
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
          {!known ? 'Pick the show to say yes.' : !saveable ? `The show has ${p.episodes} episodes.`
            : yes ? `Marked for you in ${left}` : 'Not marked unless you say yes'}
        </span>
        <span class={styles.answers}>
          <button type="button" class={yes ? styles.no : styles.save} onClick={() => store.answerWatch(p.id, false)}>No</button>
          <button type="button" class={yes ? styles.save : styles.no} disabled={!saveable}
            onClick={() => store.answerWatch(p.id, true)}>Yes, mark it</button>
        </span>
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
