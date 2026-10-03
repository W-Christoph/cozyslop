import { useRoomStore } from './RoomContext'
import { UserAvatar } from './UserAvatar'
import { userIdentity } from './UserHoverName'
import styles from './UserSidebar.module.css'

export function UserSidebar() {
  const store = useRoomStore()
  return <div class={styles.users}>
    {Array.from(store.users.value.values()).map((user) => <div class={styles.user} key={user.key}>
      <UserAvatar user={user} presence />
      <div class={styles.name}>
        <div title={userIdentity(user)} style={{ color: user.nameColor }}>{user.nickname}</div>
        <div class={styles.identity}>{userIdentity(user)}</div>
        {!user.active && <div class={styles.lastSeen}>last seen: <time dateTime={new Date(user.lastSeen).toISOString()}>{new Date(user.lastSeen).toLocaleTimeString()}</time></div>}
      </div>
    </div>)}
  </div>
}
