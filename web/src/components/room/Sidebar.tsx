import { preferences } from '../../app/state'
import { ChatPanel } from '../chat/ChatPanel'
import { useRoomStore } from './RoomContext'
import { UserSidebar } from './UserSidebar'
import styles from './Sidebar.module.css'

export type SidebarTab = 'CHAT' | 'USERS' | 'NOTHING'
export function Sidebar({ tab, fullscreen, idle }: { tab: SidebarTab; fullscreen: boolean; idle: boolean }) {
  const store = useRoomStore()
  const hidden = tab === 'NOTHING'
  const transparent = fullscreen && preferences.value.transparentChat
  return <aside hidden={hidden} aria-label={`${tab.toLowerCase()} sidebar`}
    class={`${styles.sidebar} ${transparent ? styles.transparent : ''} ${fullscreen && idle && tab === 'CHAT' ? styles.hidden : ''} ${transparent && store.isHost.value ? styles.host : ''}`}>
    <ChatPanel active={tab === 'CHAT'} />
    {tab === 'USERS' && <UserSidebar />}
  </aside>
}
