import { useLayoutEffect, useRef } from 'preact/hooks'
import { preferences } from '../../app/state'
import type { User } from '../../room/protocol'
import { applyNameColors } from '../chat/nameColor'
import { useRoomStore } from './RoomContext'
import { StreamPreview } from './StreamPreview'
import { UserAvatar } from './UserAvatar'
import { userIdentity } from './UserHoverName'
import styles from './PeoplePanel.module.css'

// The right column of the chat view, where the chat is otherwise: what is on
// the desktop, and who is in the room, grouped as in a chat app's member list.
export function PeoplePanel() {
  const store = useRoomStore()
  const panel = useRef<HTMLElement>(null)
  const { showMuted } = preferences.value
  const users = [...store.users.value.values()]
  const remote = store.remoteHolder.value
  const groups: [string, User[]][] = [
    ['Watching', users.filter((u) => u.active && !(showMuted && (u.muted || u.listening)))],
    ['Listening', users.filter((u) => u.active && showMuted && u.listening && !u.muted)],
    ['Sound off', users.filter((u) => u.active && showMuted && u.muted && !u.paused)],
    ['Paused', users.filter((u) => u.active && showMuted && u.paused)],
    ['Away', users.filter((u) => !u.active)],
  ]
  useLayoutEffect(() => applyNameColors(panel.current))
  const preview = preferences.value.streamPreview > 0 && (store.paused.value || store.audioOnly.value) && !store.desktopOffline.value
  return (
    <aside ref={panel} class={styles.panel} aria-label="People">
      {preview && <section class={styles.desktop} aria-label="On the desktop">
        <h2 class={styles.heading}>On the desktop</h2>
        <StreamPreview class={styles.preview} />
      </section>}
      {groups.filter(([, list]) => list.length).map(([name, list]) => <section key={name} aria-label={name}>
        <h2 class={styles.heading}>{name} — {list.length}</h2>
        <ul class={styles.list}>
          {list.map((user) => <li key={user.key} class={`${styles.person} ${user.active ? '' : styles.away}`}>
            <UserAvatar user={user} small />
            <div class={styles.names}>
              <span data-chat-name class={styles.name} style={{ '--name-colour': user.nameColor }}>{user.nickname}</span>
              <span class={styles.detail}>{remote === user.key ? 'Has the remote' : userIdentity(user)}</span>
            </div>
          </li>)}
        </ul>
      </section>)}
    </aside>
  )
}
