import { useEffect } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { RoomTooltip } from './RoomTooltip'
import styles from './StreamPreview.module.css'

// A small still of the desktop in the chat view while there is no picture
// (paused, or sound only), asked for every few seconds
// (preferences.streamPreview). The server shares one shrunken screenshot
// between everyone asking: a few kB each time instead of the video. Clicking
// it shows the whole stream.
export function StreamPreview({ class: className }: { class?: string }) {
  const store = useRoomStore()
  const every = preferences.value.streamPreview
  const on = every > 0 && (store.paused.value || store.audioOnly.value) && !store.desktopOffline.value && store.server.value === 'connected'
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
      <button type="button" class={`${styles.preview} ${className ?? ''}`} aria-label="Watch the stream" onClick={() => store.watch()}>
        {preview ? <img class={styles.picture} src={preview.image} alt="" /> : <span class={styles.picture} />}
        <span class={styles.play}><img src="/svg/play_button.svg" alt="" /></span>
      </button>
    </RoomTooltip>
  )
}
