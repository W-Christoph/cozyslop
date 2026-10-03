// Prototype room page: stream plus remote control. Chat, user list and the
// rest of the CozyCast UI get ported on top of this.

import { useState } from 'preact/hooks'
import { useRoute } from 'preact-iso'
import { RemoteScreen } from '../components/RemoteScreen'
import { useRoom } from '../room/useRoom'
import styles from './RoomPage.module.css'

// RoomRoute reads the room and access code from the URL: /room/<name>?access=<code>
export function RoomRoute() {
  const { params, query } = useRoute()
  return <RoomPage room={params.room} access={query.access} />
}

export function RoomPage({ room, access }: { room: string; access?: string }) {
  const r = useRoom(room, access)
  const [muted, setMuted] = useState(true)

  const status = r.kicked.value
    ? `You can't be in this room (${r.kicked.value.reason}).`
    : r.server.value !== 'connected'
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
          <button onClick={() => r.dropRemote()}>Drop remote</button>
        ) : (
          <button
            disabled={!r.rights.value.remote || r.video.value !== 'connected'}
            onClick={() => r.takeRemote()}
            title={r.rights.value.remote ? undefined : 'You are not allowed to use the remote'}
          >
            {r.remoteHolder.value ? 'Take remote' : 'Remote'}
          </button>
        )}
      </header>
      <main class={styles.screen}>
        <RemoteScreen neko={r.neko} stream={r.stream.value} isHost={r.isHost.value} muted={muted} />
      </main>
    </div>
  )
}
