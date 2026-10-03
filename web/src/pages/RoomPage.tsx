// Prototype room page: stream plus remote control. Chat, user list and the
// rest of the CozyCast UI get ported on top of this.

import { useState } from 'preact/hooks'
import { RemoteScreen } from '../components/RemoteScreen'
import { useRoom } from '../room/useRoom'
import styles from './RoomPage.module.css'

export function RoomPage({ room, name }: { room: string; name: string }) {
  const r = useRoom(room, name)
  const [muted, setMuted] = useState(true)

  const status =
    r.server.value !== 'connected'
      ? 'Connecting to server…'
      : r.error.value ?? (r.video.value === 'connected' ? 'Live' : 'Connecting to desktop…')

  return (
    <div class={styles.page}>
      <header class={styles.toolbar}>
        <strong>{room}</strong>
        <span class={styles.status}>{status}</span>
        <span class={styles.spacer} />
        <button onClick={() => setMuted(!muted)}>{muted ? 'Unmute' : 'Mute'}</button>
        {r.isHost.value ? (
          <button onClick={() => r.neko.releaseControl()}>Drop remote</button>
        ) : (
          <button
            disabled={!r.canHost.value || r.video.value !== 'connected'}
            onClick={() => r.neko.requestControl()}
            title={r.canHost.value ? undefined : 'You are not allowed to use the remote'}
          >
            {r.hasHost.value ? 'Take remote' : 'Remote'}
          </button>
        )}
      </header>
      <main class={styles.screen}>
        <RemoteScreen neko={r.neko} stream={r.stream.value} isHost={r.isHost.value} muted={muted} />
      </main>
    </div>
  )
}
