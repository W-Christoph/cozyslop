import { useEffect, useLayoutEffect, useRef } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { RoomTooltip } from './RoomTooltip'
import styles from './StreamPreview.module.css'

// A frame grabbed longer ago than this is not the stream going away.
const SHRINK_FRESH_MS = 1_500

// A small still of the desktop in the chat view while there is no picture
// (paused, or sound only), asked for every few seconds
// (preferences.streamPreview). The server shares one shrunken screenshot
// between everyone asking: a few kB each time instead of the video. Clicking
// it shows the whole stream. As the stream goes, its last frame shrinks from
// where it was into the preview.
export function StreamPreview({ class: className }: { class?: string }) {
  const store = useRoomStore()
  const button = useRef<HTMLButtonElement>(null)
  const every = preferences.value.streamPreview
  const on = every > 0 && (store.paused.value || store.audioOnly.value) && !store.desktopOffline.value && store.server.value === 'connected'
  useLayoutEffect(() => {
    const from = store.lastPicture
    store.lastPicture = null
    const el = button.current
    if (!from || !el || Date.now() - from.time > SHRINK_FRESH_MS || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const to = el.getBoundingClientRect()
    if (!to.width || !to.height) return
    // A copy of the frame over everything, flying from where the stream was
    // to the preview. Its size, not a scale: a scaled-up small picture blurs.
    const ghost = document.createElement('img')
    ghost.src = from.image
    ghost.alt = ''
    ghost.className = styles.ghost
    const place = ({ left, top, width, height }: { left: number; top: number; width: number; height: number }) =>
      ({ left: `${left}px`, top: `${top}px`, width: `${width}px`, height: `${height}px` })
    Object.assign(ghost.style, place(to))
    document.body.append(ghost)
    el.style.opacity = '0'
    const flight = ghost.animate([{ ...place(from.rect), borderRadius: '0' }, place(to)],
      { duration: 450, easing: 'cubic-bezier(0.3, 0, 0.2, 1)' })
    const land = () => { ghost.remove(); el.style.opacity = '' }
    flight.onfinish = land
    return () => { flight.cancel(); land() }
  }, [store])
  useEffect(() => {
    if (!on) return
    // A hidden tab asks for nothing, and asks right away when it is shown again.
    const ask = () => { if (!document.hidden) store.requestPreview() }
    ask()
    const timer = window.setInterval(ask, every * 1000)
    document.addEventListener('visibilitychange', ask)
    return () => { window.clearInterval(timer); document.removeEventListener('visibilitychange', ask) }
  }, [on, every, store])
  if (!on) return null
  const preview = store.preview.value
  return (
    <RoomTooltip label="Watch the stream">
      <button ref={button} type="button" class={`${styles.preview} ${className ?? ''}`} aria-label="Watch the stream" onClick={() => store.watch()}>
        {preview ? <img class={styles.picture} src={preview.image} alt="" /> : <span class={styles.picture} />}
        <span class={styles.play}><img src="/svg/play_button.svg" alt="" /></span>
      </button>
    </RoomTooltip>
  )
}
