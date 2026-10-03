import { preferences } from '../../app/state'
import { ChatPanel } from '../chat/ChatPanel'
import { RoomAdminPanel } from './admin/RoomAdminPanel'
import { useRoomStore } from './RoomContext'
import { UserSidebar } from './UserSidebar'
import styles from './Sidebar.module.css'

export type SidebarTab = 'CHAT' | 'USERS' | 'SETTINGS' | 'NOTHING'
export function Sidebar({ tab, fullscreen, idle }: { tab: SidebarTab; fullscreen: boolean; idle: boolean }) {
  const store = useRoomStore()
  if (tab === 'NOTHING' || (tab === 'SETTINGS' && !store.rights.value.admin)) return null
  const transparent = fullscreen && preferences.value.transparentChat
  return <aside aria-label={`${tab.toLowerCase()} sidebar`}
    class={`${styles.sidebar} ${transparent ? styles.transparent : ''} ${fullscreen && idle && tab === 'CHAT' ? styles.hidden : ''} ${transparent && store.isHost.value ? styles.host : ''}`}>
    {tab === 'CHAT' ? <ChatPanel /> : tab === 'USERS' ? <UserSidebar /> : <RoomAdminPanel />}
  </aside>
}
