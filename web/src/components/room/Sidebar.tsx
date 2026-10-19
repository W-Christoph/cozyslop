import { preferences } from '../../app/state'
import { ChatPanel } from '../chat/ChatPanel'
import type { TemporaryLine } from '../chat/useChatEvents'
import { useRoomStore } from './RoomContext'
import styles from './Sidebar.module.css'

export function Sidebar({ chatOpen, lines, fullscreen, idle }: { chatOpen: boolean; lines: readonly TemporaryLine[]; fullscreen: boolean; idle: boolean }) {
  const store = useRoomStore()
  const transparent = fullscreen && preferences.value.transparentChat
  return <aside hidden={!chatOpen} aria-label="Chat"
    class={`${styles.sidebar} ${transparent ? styles.transparent : ''} ${transparent && store.isHost.value ? styles.host : ''}`}>
    <ChatPanel lines={lines} active={chatOpen} idle={fullscreen && idle} />
  </aside>
}
