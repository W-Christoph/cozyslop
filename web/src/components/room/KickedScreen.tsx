import { InfoScreen } from '../InfoScreen'
import { ButtonLink } from '../Button'
import { Header } from '../Header'
import type { IconName } from '../ui/Icon'
import { useRoomStore } from './RoomContext'
import styles from './KickedScreen.module.css'

export function KickedScreen() {
  const store = useRoomStore()
  const kick = store.kicked.value
  if (!kick) return null
  const info = ({
    banned: ['You are banned', kick.bannedUntil ? `until ${new Date(kick.bannedUntil * 1000).toLocaleString()}` : 'permanently', 'ban'],
    account: ['You are not allowed in this room', 'Please log in to join this room.', 'lock'],
    verified: ['You are not allowed in this room', 'Only verified accounts may join this room.', 'shield'],
    invite: ['You are not allowed in this room', 'This room is invite only.', 'ticket'],
    kicked: ['You have been kicked', 'You were removed from this room.', 'logout'],
    deleted: ['Account deleted or disabled', 'Your account is no longer available.', 'user'],
    not_found: ['Room not found', 'This room does not exist.', 'search'],
    room_changed: ['Room updated', 'The room’s connection details changed. Reopen the room to reconnect.', 'logout'],
    session: ['Session expired', 'Please log in again to join this room.', 'lock'],
  } satisfies Record<typeof kick.reason, [string, string, IconName]>)[kick.reason]
  const login = kick.reason === 'account' || kick.reason === 'session'
  return <div class={styles.page}>
    <Header />
    <InfoScreen message={info[0]} submessage={info[1]} icon={info[2]}>
      {login && <ButtonLink variant="primary" size="lg" href="/login">Log in</ButtonLink>}
      <ButtonLink variant={login ? 'secondary' : 'primary'} size="lg" href="/">Back to rooms</ButtonLink>
    </InfoScreen>
  </div>
}
