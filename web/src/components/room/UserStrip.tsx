import { useEffect, useLayoutEffect, useRef } from 'preact/hooks'
import { preferences } from '../../app/state'
import type { User } from '../../room/protocol'
import { useRoomStore } from './RoomContext'
import { UserAvatar } from './UserAvatar'
import { userIdentity, type HoverName } from './UserHoverName'
import { awayTime } from './UserCard'
import styles from './UserStrip.module.css'

export function UserStrip({ left, fullscreen, hover, onHover }: { left: boolean; fullscreen: boolean; hover: HoverName | null; onHover: (hover: HoverName | null) => void }) {
  const store = useRoomStore()
  const { smallPfp, showUsernames, showIfMuted } = preferences.value
  const pinned = useRef<HTMLDivElement | null>(null)
  const touch = useRef(false)
  useLayoutEffect(() => { if (!hover) pinned.current = null }, [hover])
  useEffect(() => {
    const dismiss = (e: PointerEvent) => {
      touch.current = e.pointerType === 'touch'
      if (pinned.current && !pinned.current.contains(e.target as Node)) {
        pinned.current = null
        onHover(null)
      }
    }
    const keyboard = (e: KeyboardEvent) => {
      touch.current = false
      pinned.current = null
      if (e.key === 'Escape') onHover(null)
    }
    document.addEventListener('pointerdown', dismiss, true)
    document.addEventListener('keydown', keyboard, true)
    return () => {
      document.removeEventListener('pointerdown', dismiss, true)
      document.removeEventListener('keydown', keyboard, true)
    }
  }, [onHover])
  const showHover = (e: { currentTarget: HTMLDivElement }, user: User) => {
    const rect = e.currentTarget.getBoundingClientRect()
    onHover({ user, left, x: left ? rect.right + rect.width * 0.1 : rect.left + rect.width / 2,
      y: left ? rect.top + rect.height / 2 : rect.top - 8 })
  }
  return (
    <div class={`${styles.strip} ${left ? styles.left : styles.bottom} ${smallPfp ? styles.small : ''} ${showUsernames ? styles.names : ''} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && store.isHost.value ? styles.host : ''}`}
      role="list" aria-label="Room users">
      {Array.from(store.users.value.values()).map((user) => <div class={styles.user} role="listitem" key={user.key}>
        <div class={styles.avatar} tabIndex={0} aria-label={[user.nickname, userIdentity(user),
          !user.active && `away since ${awayTime(user.lastSeen)}`, store.remoteHolder.value === user.key && 'has the remote',
          showIfMuted && user.muted && 'sound off'].filter(Boolean).join(', ')}
          onPointerEnter={(e) => { if (e.pointerType === 'mouse' && !pinned.current) showHover(e, user) }}
          onPointerLeave={() => { if (!pinned.current) onHover(null) }}
          onPointerDown={(e) => {
            if (e.pointerType !== 'touch') return
            if (pinned.current === e.currentTarget) { pinned.current = null; onHover(null) }
            else { pinned.current = e.currentTarget; showHover(e, user) }
          }}
          onFocus={(e) => { if (!touch.current) showHover(e, user) }} onBlur={() => { if (!pinned.current) onHover(null) }}>
          <UserAvatar user={user} small={smallPfp} />
        </div>
        {showUsernames && !(smallPfp && left) && <div class={`${styles.name} ${user.active ? '' : styles.away}`}>{user.nickname}</div>}
      </div>)}
    </div>
  )
}
