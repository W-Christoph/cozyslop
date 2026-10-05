import { useCallback, useRef, useState } from 'preact/hooks'
import { MobileRemoteControls } from './MobileRemoteControls'
import { RemoteScreen } from './RemoteScreen'
import { DesktopUploadStatus, useDesktopDrop } from './DesktopUpload'
import { useRoomStore } from './RoomContext'
import styles from './VideoArea.module.css'

// Match CozyCast's device test, including tablet browsers.
const mobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent)

export function VideoArea({ disconnected, error, fullscreen }: { disconnected: boolean; error: string; fullscreen: boolean }) {
  const store = useRoomStore()
  const drop = useDesktopDrop()
  const pointer = useRef({ x: store.neko.screen.width / 2, y: store.neko.screen.height / 2 })
  const [blocked, setBlocked] = useState(false)
  const video = useRef<HTMLVideoElement>(null)
  const onPlaybackBlocked = useCallback((value: boolean) => setBlocked(value), [])
  const paused = store.paused.value
  const restarting = store.restarting.value
  const loading = restarting !== null || store.server.value !== 'connected' || store.video.value !== 'connected'
  const title = disconnected ? 'Connection lost' : restarting !== null ? `${restarting} restarted the room`
    : store.server.value !== 'connected' ? 'Connecting to server…' : 'Connecting to the desktop…'
  const detail = store.error.value ?? (disconnected || restarting !== null ? 'Reconnecting…' : '')
  const play = () => {
    if (store.paused.value) {
      store.resume()
      return
    }
    // Call play in the gesture handler so restrictive mobile browsers can
    // unlock playback; an effect alone may lose the browser's activation.
    void video.current?.play().then(() => setBlocked(false)).catch(() => setBlocked(true))
  }
  return (
    <div class={`${styles.area} ${fullscreen ? '' : styles.fitted}`}>
      <main class={styles.screen} aria-label="Room screen" {...drop}>
        <DesktopUploadStatus />
        {error && <div role="alert" class={styles.error}>{error}</div>}
        <RemoteScreen mobile={mobile} pointer={pointer} video={video} onPlaybackBlocked={onPlaybackBlocked} />
        {paused && <span class={styles.status} role="status">Paused</span>}
        {!disconnected && (paused || blocked) ? (
          <button class={styles.play} aria-label={blocked ? 'Play stream' : 'Resume stream'} onClick={play}>
            <img src="/svg/initial_play_button.svg" alt="" />
          </button>
        ) : loading ? (
          <div class={`${styles.state} ${disconnected ? styles.disconnected : ''}`} role="status">
            <img src="/svg/loading-cozy.svg" alt="" />
            <span class={styles.title}>{title}</span>
            {detail && <span class={styles.detail}>{detail}</span>}
          </div>
        ) : store.audioOnly.value ? (
          <div class={`${styles.state} ${styles.audio}`} role="status">
            <img src="/svg/volume-up.svg" alt="" />
            <span class={styles.title}>Sound only</span>
            <span class={styles.detail}>The picture is off. Turn it back on in Settings › Room.</span>
          </div>
        ) : null}
      </main>
      {mobile && store.isHost.value && <MobileRemoteControls pointer={pointer} />}
    </div>
  )
}
