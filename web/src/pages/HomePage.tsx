import { useEffect, useState } from 'preact/hooks'
import { api, type RoomInfo } from '../api'
import { me, serverSettings } from '../app/state'
import { Button, ButtonLink } from '../components/Button'
import { PageLayout } from '../components/PageLayout'
import { InviteModal } from '../components/admin/InviteModal'
import { Badge } from '../components/ui/Badge'
import { EmptyState } from '../components/ui/EmptyState'
import { Spinner } from '../components/ui/Spinner'
import { Icon, type IconName } from '../components/ui/Icon'
import { Notice } from '../components/ui/Notice'
import styles from './HomePage.module.css'

const access: Record<RoomInfo['access'], { label: string; icon: IconName } | null> = {
  invite: { label: 'Invite only', icon: 'ticket' },
  account: { label: 'Accounts only', icon: 'user' },
  verified: { label: 'Verified only', icon: 'shield' },
  public: null,
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
    <PageLayout narrow title="Rooms" subtitle="Pick a room and watch together.">
      {inviteRoom !== null && (
        <InviteModal room={inviteRoom} onClose={() => setInviteRoom(null)} />
      )}
      {serverSettings.value.message && (
        <div class={styles.announcement}>
          <Icon name="info" size={18} />
          <p>{serverSettings.value.message}</p>
        </div>
      )}
      {error && <Notice tone="error">{error}</Notice>}
      {loading && rooms.length === 0 && <Spinner label="Loading rooms…" />}
      {!loading && !error && rooms.length === 0 && (
        <EmptyState icon="monitor" title="No rooms available">
          {me.value ? 'There is no room you can join right now.' : 'Log in to see the rooms you have access to.'}
        </EmptyState>
      )}
      <ul class={styles.rooms}>
        {rooms.map((room) => {
          const badge = access[room.access]
          return (
            <li class={`${styles.room} ${room.open ? '' : styles.closed}`} key={room.name}>
              <div class={styles.tile}><Icon name="monitor" size={22} /></div>
              <div class={styles.info}>
                <div class={styles.nameLine}>
                  <span class={styles.name}>{room.name}</span>
                  {badge && <Badge icon={badge.icon}>{badge.label}</Badge>}
                </div>
                <div class={styles.count}>
                  <span class={`${styles.dot} ${room.online ? styles.live : ''}`} title={room.online ? 'Desktop online' : 'Desktop offline'} />
                  {room.online
                    ? room.userCount === 0 ? 'Nobody watching' : `${room.userCount} watching`
                    : room.userCount === 0 ? 'Offline' : `Offline · ${room.userCount} in the room`}
                </div>
              </div>
              <div class={styles.actions}>
                {me.value?.admin && (
                  <Button variant="ghost" icon="ticket" onClick={() => setInviteRoom(room.name)}>
                    Invite
                  </Button>
                )}
                {room.open ? (
                  <ButtonLink variant="primary" href={`/room/${encodeURIComponent(room.name)}`}>
                    Join
                  </ButtonLink>
                ) : (
                  <Button disabled>
                    Closed
                  </Button>
                )}
              </div>
            </li>
          )
        })}
      </ul>
    </PageLayout>
  )
}
