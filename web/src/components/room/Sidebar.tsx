import { preferences } from '../../app/state'
import { ChatPanel } from '../chat/ChatPanel'
import { useRoomStore } from './RoomContext'
import styles from './Sidebar.module.css'

export function Sidebar({ chatOpen, fullscreen, idle }: { chatOpen: boolean; fullscreen: boolean; idle: boolean }) {
  const store = useRoomStore()
  const transparent = fullscreen && preferences.value.transparentChat
  return <aside hidden={!chatOpen} aria-label="Chat"
    class={`${styles.sidebar} ${transparent ? styles.transparent : ''} ${transparent && store.isHost.value ? styles.host : ''}`}>
    <ChatPanel active={chatOpen} idle={fullscreen && idle} />
  </aside>
}
