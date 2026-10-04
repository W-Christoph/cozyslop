import { useContext, useState } from 'preact/hooks'
import { logout, me, settingsOpen, type SettingsSection } from '../../app/state'
import { RoomContext } from '../room/RoomContext'
import { Avatar } from '../ui/Avatar'
import { Notice } from '../ui/Notice'
import { SettingsWindow, type NavGroup } from '../ui/SettingsLayout'
import { AccountSection } from './AccountSection'
import { AppearanceSection } from './AppearanceSection'
import { ChatSection } from './ChatSection'
import { NotificationsSection } from './NotificationsSection'
import { RoomSection } from './RoomSection'
import styles from './SettingsDialog.module.css'

// Personal settings: the account and how CozyCast looks and behaves in this
// browser. Preferences apply as they are changed.
export function SettingsDialog({ onClose }: { onClose?: () => void }) {
  const section = settingsOpen.value
  const room = useContext(RoomContext)
  const [error, setError] = useState('')
  if (!section) return null
  const user = me.value
  const close = () => {
    settingsOpen.value = null
    onClose?.()
  }
  const nav: NavGroup[] = [
    { label: 'User', items: [{ id: 'account', label: 'My account', icon: 'user' }] },
    {
      label: 'App',
      items: [
        { id: 'appearance', label: 'Appearance', icon: 'palette' },
        { id: 'chat', label: 'Chat', icon: 'message' },
        { id: 'room', label: 'Room', icon: 'monitor' },
        { id: 'notifications', label: 'Notifications', icon: 'bell' },
      ],
    },
    { items: [user ? { id: 'logout', label: 'Log out', icon: 'logout', danger: true } : { id: 'login', label: 'Log in', icon: 'login', href: '/login' }] },
  ]
  async function select(id: string) {
    setError('')
    if (id !== 'logout') {
      settingsOpen.value = id as SettingsSection
      return
    }
    try {
      await logout()
      // Closed in the meantime: a finished request must not reopen it.
      if (settingsOpen.value) settingsOpen.value = 'account'
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    }
  }
  return (
    <SettingsWindow nav={nav} current={section} onSelect={(id) => { void select(id) }} onClose={close}
      navHeader={
        <div class={styles.user}>
          <Avatar src={user?.avatarUrl} size={40} />
          <div class={styles.userText}>
            <div class={styles.name}>{user ? user.nickname : 'Guest'}</div>
            <div class={styles.username}>{user ? user.username : 'Not logged in'}</div>
          </div>
        </div>
      }
>
      {error && <div class={styles.error}><Notice tone="error">{error}</Notice></div>}
      {section === 'account' && <AccountSection />}
      {section === 'appearance' && <AppearanceSection />}
      {section === 'chat' && <ChatSection />}
      {section === 'room' && <RoomSection room={room} />}
      {section === 'notifications' && <NotificationsSection />}
    </SettingsWindow>
  )
}
