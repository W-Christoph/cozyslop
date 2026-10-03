import { useEffect, useState } from 'preact/hooks'
import { api, type RoomInfo } from '../api'
import { me, serverSettings } from '../app/state'
import { Button } from '../components/Button'
import { InviteModal } from '../components/admin/InviteModal'
import styles from './HomePage.module.css'

const badges = {
  invite: 'Invite Only',
  account: 'Account Only',
  verified: 'Verified Only',
  public: '',
}
export function HomePage() {
  const [rooms, setRooms] = useState<RoomInfo[]>([]),
    [inviteRoom, setInviteRoom] = useState<string | null>(null)
  const [error, setError] = useState(''),
    [loading, setLoading] = useState(true)
  const username = me.value?.username
  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void api
      .get<RoomInfo[]>('/api/rooms')
      .then((res) => {
        if (active) setRooms(res)
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : 'Something went wrong.')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [username])
  return (
    <main class={styles.background}>
      {inviteRoom !== null && (
        <InviteModal room={inviteRoom} onClose={() => setInviteRoom(null)} />
      )}
      <div class={styles.list}>
        {serverSettings.value.message && (
          <div class={styles.message}>{serverSettings.value.message}</div>
        )}
        <div class={styles.title}>Rooms</div>
        <table
          class={`${styles.table} ${me.value?.admin ? styles.adminTable : ''}`}
        >
          <colgroup>
            {me.value?.admin && <col class={styles.inviteColumn} />}
            <col class={styles.nameColumn} />
            <col class={styles.countColumn} />
            <col class={styles.joinColumn} />
          </colgroup>
          <tbody>
            {rooms.map((room) => (
              <tr key={room.name}>
                {me.value?.admin && (
                  <td>
                    <Button onClick={() => setInviteRoom(room.name)}>
                      Invite
                    </Button>
                  </td>
                )}
                <td>
                  <span class={styles.name}>{room.name}</span>
                  {badges[room.access] && (
                    <span class={styles.badge}>{badges[room.access]}</span>
                  )}
                </td>
                <td>{room.userCount} users</td>
                <td class={styles.joinCell}>
                  {room.open ? (
                    <a
                      class={styles.join}
                      href={`/room/${encodeURIComponent(room.name)}`}
                    >
                      Join
                    </a>
                  ) : (
                    <Button class={styles.closed} disabled>
                      Closed
                    </Button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {loading && <div role="status">Loading rooms...</div>}
        {error && <p role="alert">{error}</p>}
        {!loading && !error && rooms.length === 0 && (
          <div>Currently none available</div>
        )}
      </div>
    </main>
  )
}
