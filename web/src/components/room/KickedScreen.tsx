import { InfoScreen } from '../InfoScreen'
import { useRoomStore } from './RoomContext'
import styles from './KickedScreen.module.css'

export function KickedScreen() {
  const store = useRoomStore()
  const kick = store.kicked.value
  if (!kick) return null
  const info = {
    banned: ['You are banned', kick.bannedUntil ? `until ${new Date(kick.bannedUntil * 1000).toLocaleString()}` : 'permanently'],
    account: ['You are not allowed in this room', 'Please log in to join this room.'],
    verified: ['You are not allowed in this room', 'Only verified accounts may join this room.'],
    invite: ['You are not allowed in this room', 'This room is invite only.'],
    kicked: ['You have been kicked', 'You were removed from this room.'],
    deleted: ['Account deleted or disabled', 'Your account is no longer available.'],
  }[kick.reason]
  return <InfoScreen message={info[0]} submessage={info[1]}>
    {kick.reason === 'account' && <a class={styles.link} href="/login">Login</a>}
    <a class={styles.link} href="/">Home</a>
  </InfoScreen>
}
