import { useContext, useState } from 'preact/hooks'
import { logout, me, settingsOpen, type SettingsSection } from '../../app/state'
import { Button } from '../Button'
import { Modal } from '../Modal'
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
  const [dirty, setDirty] = useState(false)
  const [pending, setPending] = useState<(() => void) | null>(null)
  const [profileVersion, setProfileVersion] = useState(0)
  if (!section) return null
  const user = me.value
  const close = () => {
    settingsOpen.value = null
    onClose?.()
  }
  function leave(action: () => void) {
    if (section === 'account' && dirty) setPending(() => action)
    else action()
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
    <>
      <SettingsWindow nav={nav} current={section} onSelect={(id) => { if (id !== section) leave(() => { void select(id) }) }} onClose={() => leave(close)}
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
        {section === 'account' && <AccountSection onDirtyChange={setDirty} profileVersion={profileVersion} />}
        {section === 'appearance' && <AppearanceSection />}
        {section === 'chat' && <ChatSection />}
        {section === 'room' && <RoomSection room={room} />}
        {section === 'notifications' && <NotificationsSection />}
      </SettingsWindow>
      {pending && <Modal size="sm" title="Discard profile changes?" onClose={() => setPending(null)} footer={<>
        <Button onClick={() => setPending(null)}>Keep editing</Button>
        <Button variant="danger" onClick={() => {
          setPending(null)
          setDirty(false)
          setProfileVersion((version) => version + 1)
          pending()
        }}>Discard</Button>
      </>}>
        <p>Your nickname, name colour and new avatar have not been saved.</p>
      </Modal>}
    </>
  )
}
