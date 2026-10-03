import { preferences } from '../../app/state'
import type { User } from '../../room/protocol'
import { useRoomStore } from './RoomContext'
import { UserAvatar } from './UserAvatar'
import { userIdentity, type HoverName } from './UserHoverName'
import styles from './UserStrip.module.css'

export function UserStrip({ left, fullscreen, onHover }: { left: boolean; fullscreen: boolean; onHover: (hover: HoverName | null) => void }) {
  const store = useRoomStore()
  const { smallPfp, showUsernames } = preferences.value
  const showHover = (e: { currentTarget: HTMLDivElement }, user: User) => {
    const rect = e.currentTarget.getBoundingClientRect()
    onHover({ user, left, x: left ? rect.right + rect.width * 0.1 : rect.left + rect.width / 2,
      y: left ? rect.top + rect.height / 2 : rect.top - 8 })
  }
  return (
    <div class={`${styles.strip} ${left ? styles.left : styles.bottom} ${smallPfp ? styles.small : ''} ${showUsernames ? styles.names : ''} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && store.isHost.value ? styles.host : ''}`}
      aria-label="Room users">
      {Array.from(store.users.value.values()).map((user) => <div class={styles.user} key={user.key}>
        <div class={styles.avatar} tabIndex={0} aria-label={`${user.nickname}, ${userIdentity(user)}, ${user.active ? 'online' : 'away'}`}
          onMouseEnter={(e) => showHover(e, user)} onMouseLeave={() => onHover(null)}
          onFocus={(e) => showHover(e, user)} onBlur={() => onHover(null)}>
          <UserAvatar user={user} small={smallPfp} />
        </div>
        {showUsernames && !(smallPfp && left) && <div class={`${styles.name} ${user.active ? '' : styles.away}`}>{user.nickname}</div>}
      </div>)}
    </div>
  )
}
