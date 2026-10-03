import { useState } from 'preact/hooks'
import { me, preferences, updatePreferences, type Preferences, type Theme } from '../../app/state'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { ProfileEditor } from '../profile/ProfileEditor'
import { useRoomStore } from './RoomContext'
import styles from './PersonalSettings.module.css'

type ToggleKey = Exclude<keyof Preferences, 'theme' | 'volume'>
const groups: { title: string; fields: [ToggleKey, string][] }[] = [
  { title: 'Notification', fields: [
    ['muteChatNotification', 'Mute Chat Notification'],
    ['showIfMuted', 'Show Others If Muted'],
    ['showLeaveJoinMsg', 'Show Leave/Join Message'],
    ['titleNameInFront', 'Display CozyCast In Title First'],
  ] },
  { title: 'Userlist', fields: [
    ['userlistOnLeft', 'Show Users On Left'],
    ['showUsernames', 'Show Usernames'],
    ['smallPfp', 'Use Small Profile Pictures'],
  ] },
  { title: 'Media', fields: [
    ['manualLoadMedia', 'Manually Load Images and Videos'],
    ['audioOnly', 'Stream Music Only'],
  ] },
]

export function PersonalSettings({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const [draft, setDraft] = useState(() => ({ ...preferences.peek() }))
  const [message, setMessage] = useState('')
  const toggle = (field: ToggleKey, value: boolean) => {
    setDraft((previous) => ({ ...previous, [field]: value }))
    setMessage('')
  }
  const apply = () => {
    // Keep any volume/mute changes made while this modal was open.
    const { volume: _volume, muted: _muted, ...changes } = draft
    updatePreferences(changes)
    store.setAudioOnly(draft.audioOnly)
    setMessage('Settings applied!')
  }
  return <Modal title="SETTINGS" onClose={onClose}>
    <div class={styles.settings}>
      <details>
        <summary>Edit Profile</summary>
        <div class={styles.profile}>{me.value ? <ProfileEditor /> : <p>Please log in to edit your profile. <a href="/login">Login</a></p>}</div>
      </details>
      {groups.map(({ title, fields }) => <details key={title}>
        <summary>{title}</summary>
        <div class={styles.fields}>{fields.map(([field, description]) => <label key={field}>
          <input type="checkbox" checked={draft[field]} onChange={(e) => toggle(field, e.currentTarget.checked)} />
          {description}
        </label>)}</div>
      </details>)}
      <details>
        <summary>Design</summary>
        <div class={styles.fields}>
          <label><input type="checkbox" checked={draft.transparentChat} onChange={(e) => toggle('transparentChat', e.currentTarget.checked)} />Fullscreen Transparent Chat</label>
          <label>Theme <select aria-label="Theme" value={draft.theme} onChange={(e) => { const theme = e.currentTarget.value as Theme; setDraft((previous) => ({ ...previous, theme })); setMessage('') }}>
            <option value="default">Default</option><option value="legacy">Legacy</option><option value="light">Light</option>
          </select></label>
        </div>
      </details>
    </div>
    <div class={styles.actions}><Button accent onClick={apply}>Apply</Button><Button onClick={onClose}>Close</Button></div>
    <div class={styles.message} role="status">{message}</div>
  </Modal>
}
