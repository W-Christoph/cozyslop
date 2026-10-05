import type { User } from '../../room/protocol'
import { useRoomStore } from './RoomContext'
import styles from './UserHoverName.module.css'

export function userIdentity(user: Pick<User, 'key' | 'anonymous' | 'username'> | string): string {
  const key = typeof user === 'string' ? user : user.key
  const anonymous = typeof user === 'string' ? key.startsWith('a:') : user.anonymous
  return anonymous ? `Anon(${key.replace(/^a:/, '').slice(0, 4)})` : typeof user === 'string' ? key : user.username
}

export interface HoverName { user: User; x: number; y: number; left: boolean }
export function UserHoverName({ hover }: { hover: HoverName | null }) {
  const store = useRoomStore()
  if (!hover) return null
  const { x, y, left } = hover
  const user = store.users.value.get(hover.user.key)
  if (!user) return null
  return <div role="tooltip" class={`${styles.tooltip} ${left ? styles.right : styles.top} ${user.active ? styles.online : styles.away}`}
    style={{ left: x, top: y }}>
    <strong>{user.nickname}</strong>
    <span>{userIdentity(user)}</span>
  </div>
}
