import { useEffect, useState } from 'preact/hooks'
import type { WatchProposal, WatchViewer } from '../../room/protocol'
import { AnimeSearch, Cover } from '../watchlog/AnimeSearch'
import { Checkbox } from '../ui/Field'
import { Icon } from '../ui/Icon'
import { useRoomStore, watchlogWanted } from './RoomContext'
import styles from './WatchPanel.module.css'

// The episode VLC or Firefox plays, as a bar at the top of the chat: the
// show's whole name, on two lines if it needs them. Accounts get to the
// watchlog from it, and say which show it is where that was not recognised
// ("Which show?") or is AniList's guess, which may be wrong (a small button): the
// name it goes by is that show's from then on, and the button is gone.
function WatchPlayingBar() {
  const store = useRoomStore()
  const playing = store.watchPlaying.value
  const [linking, setLinking] = useState(false)
  if (!playing) return null
  const known = playing.animeId !== 0
  const state = !known ? 'Not recognised' : playing.linked ? 'Watching' : 'Best guess'
  const line = <>
    <Icon name="play" size={14} />
    <span class={styles.playingText}>
      <span class={styles.label}>{known ? 'Watching' : 'Watching · unknown show'}</span>
      <strong class={styles.show} title={playing.title}>{playing.title}</strong>
    </span>
    <span class={styles.ep}>Ep {playing.episode}</span>
  </>
  if (store.self.value?.anonymous !== false) return <div role="status"><div class={styles.playing}>{line}</div></div>
  // The buttons are not in the live region: only what plays is announced.
  return (
    <div>
      <div class={styles.bar}>
        <button type="button" class={`${styles.playing} ${styles.open}`} title="Open the watchlog"
          aria-label={`${state}: ${playing.title}, episode ${playing.episode}. Opens the watchlog`} onClick={() => { watchlogWanted.value = true }}>{line}</button>
        {!playing.linked && (
          <button type="button" class={`${styles.linkShow} ${known ? '' : styles.wanted}`} aria-expanded={linking}
            aria-label={known ? 'Wrong show? Pick the right one' : 'Which show is this? Pick it'}
            title={known ? 'Wrong show? Pick the right one' : 'Say which show this is. Its other episodes are then recognised.'}
            onClick={() => setLinking(!linking)}><Icon name="link" size={14} />{!known && 'Which show?'}</button>
        )}
        <span class={styles.hidden} role="status">{`${state}: ${playing.title}, episode ${playing.episode}`}</span>
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
// The suggested answer is the highlighted button. Who has not marked the
// episodes before it (seen elsewhere, before the watchlog) may tick those
// to be marked too: a choice, taken back by unticking, that is carried out
// with their yes.
function WatchAsk({ proposal: p, viewer }: { proposal: WatchProposal; viewer: WatchViewer }) {
  const store = useRoomStore()
  const [fixing, setFixing] = useState(false)
  const left = useCountdown(p.applyAt)
  const max = p.episodes ?? 9999
  const known = p.animeId !== 0
  // Yes needs a show, and an episode it has.
  const saveable = known && p.episode <= max
  const yes = viewer.checked && saveable
  // "1–8", or how many of them where some are marked.
  const before = p.episode - 1
  const all = viewer.earlier === before
  const earlier = all ? (before === 1 ? 'ep 1' : `eps 1–${before}`)
    : `${viewer.earlier} earlier ep${viewer.earlier === 1 ? '' : 's'}`
  const said = `watch-default-${p.id}` // what happens without an answer, for the buttons
  return (
    <section class={styles.card} aria-label={known ? `Did you watch ${p.title} episode ${p.episode}?` : 'Which show did you watch?'}>
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
          {known && !fixing && <button type="button" class={styles.link} onClick={() => setFixing(true)}>Wrong show or episode?</button>}
        </div>
      </div>
      {!known && <p class={styles.file} title={p.file}>{p.file}</p>}
      {(fixing || !known) && <AnimeSearch autoFocus={fixing} placeholder="Search AniList…" onPick={(anime) => { store.setWatchAnime(p.id, anime.id); setFixing(false) }} />}
      <div class={styles.foot}>
        {/* Short, to stay on one line beside the buttons; the whole of it
            for screen readers and on hover. */}
        <span class={`${styles.muted} ${styles.status}`} id={said}
          title={yes ? `Answers yes in ${left} unless you say no` : undefined}>
          {!known ? 'Pick the show to mark it' : !saveable ? `The show has ${p.episodes} episodes`
            : yes ? <><span aria-hidden="true">Yes in </span><span class={styles.hidden}>Answers yes unless you say no, in </span><span class={styles.time}>{left}</span></>
            : 'No unless you say yes'}
        </span>
        <span class={styles.answers}>
          <button type="button" class={yes || !known ? styles.no : styles.save} aria-describedby={said} onClick={() => store.answerWatch(p.id, false)}>No</button>
          <button type="button" class={yes ? styles.save : styles.no} disabled={!saveable} aria-describedby={said}
            onClick={() => store.answerWatch(p.id, true)}>Yes, mark it</button>
        </span>
      </div>
      {saveable && viewer.earlier > 0 && (
        <label class={styles.earlier} title={`Marked with episode ${p.episode} when you say yes`}>
          <Checkbox checked={viewer.earlierToo} onChange={(e) => store.setWatchEarlier(p.id, e.currentTarget.checked)} />
          <span>Also mark <span class={styles.time}>{earlier}</span> <span class={styles.muted}>(not marked yet)</span></span>
        </label>
      )}
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
