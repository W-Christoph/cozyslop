import { useLayoutEffect, useRef } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { userIdentity, type HoverName } from './UserHoverName'
import surface from '../ui/RoomTooltipSurface.module.css'
import styles from './UserCard.module.css'

export function awayTime(lastSeen: number) {
  return new Date(lastSeen).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

export function UserCard({ hover }: { hover: HoverName | null }) {
  const store = useRoomStore()
  const card = useRef<HTMLDivElement>(null)
  const user = hover && store.users.value.get(hover.user.key)
  const remote = user && store.remoteHolder.value === user.key
  const muted = user && preferences.value.showIfMuted && user.muted
  useLayoutEffect(() => {
    if (!hover || !card.current) return
    const clamp = () => {
      const element = card.current
      if (!element) return
      element.style.left = `${hover.x}px`
      const rect = element.getBoundingClientRect()
      const x = Math.max(8, Math.min(rect.left, window.innerWidth - rect.width - 8))
      element.style.left = `${hover.x + x - rect.left}px`
      element.style.setProperty('--arrow-x', `${hover.x - x}px`)
    }
    clamp()
    window.addEventListener('resize', clamp)
    return () => window.removeEventListener('resize', clamp)
  }, [hover, user, remote, muted])
  if (!hover || !user) return null
  return <div ref={card} role="tooltip" class={`${surface.surface} ${styles.tooltip} ${hover.left ? surface.right : surface.top} ${user.active ? styles.online : styles.away}`}
    style={{ left: hover.x, top: hover.y }}>
    <strong>{user.nickname}</strong>
    <span>{userIdentity(user)}</span>
    {!user.active && <span>Away since <time dateTime={new Date(user.lastSeen).toISOString()}>{awayTime(user.lastSeen)}</time></span>}
    {remote && <span>Has the remote</span>}
    {muted && <span>Sound off</span>}
  </div>
}
