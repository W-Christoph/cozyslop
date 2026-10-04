import { Button } from '../../Button'
import type { Access, RoomSettings } from '../../../room/protocol'
import { Spinner } from '../../ui/EmptyState'
import { Notice } from '../../ui/Notice'
import { RadioCards, type RadioOption } from '../../ui/RadioCards'
import { Section, ToggleRow } from '../../ui/Section'
import { useRoomSettingsForm } from './useRoomSettingsForm'
import styles from './RoomAccessSettings.module.css'

const accessOptions: RadioOption<Access>[] = [
  { value: 'public', label: 'Public', description: 'Anyone with the link, account or not.' },
  { value: 'account', label: 'Users', description: 'Anyone who is logged in.' },
  { value: 'verified', label: 'Verified users', description: 'Accounts an admin has verified.' },
  { value: 'invite', label: 'Invited users', description: 'Accounts invited to this room.' },
]
const defaults = [
  ['defaultRemote', 'Use the remote', 'Control the desktop with mouse and keyboard.'],
  ['defaultImage', 'Post images in chat', 'Pictures, videos and screenshots.'],
  ['defaultUpload', 'Upload files to the desktop', "Also lets them download from the desktop's Downloads folder."],
] as const satisfies readonly (readonly [keyof RoomSettings, string, string])[]
const fields = ['access', 'hidden', 'remoteOwnership', ...defaults.map(([key]) => key)] as const

export function RoomAccessSettings() {
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  if (!draft) return <Spinner label="Loading room settings..." />
  return (
    <form onSubmit={(e) => { e.preventDefault(); void save() }}>
      <Section title="Who can join">
        <RadioCards name="roomAccess" label="Access" columns={2} value={draft.access} disabled={busy}
          options={accessOptions} onChange={(value) => change('access', value)} />
        <ToggleRow title="Hidden to unauthorized" description="Only people who may join see this room in the list."
          checked={draft.hidden} disabled={busy} onChange={(value) => change('hidden', value)} />
      </Section>
      <Section title="Default permissions" description="What everyone in the room may do without a permission of their own.">
        {defaults.map(([key, title, description]) => (
          <ToggleRow key={key} title={title} description={description} checked={draft[key]} disabled={busy}
            onChange={(value) => change(key, value)} />
        ))}
      </Section>
      <Section title="Remote">
        <ToggleRow title="Remote ownership" description="Whoever holds the remote keeps it until they drop it; others cannot take it away."
          checked={draft.remoteOwnership} disabled={busy} onChange={(value) => change('remoteOwnership', value)} />
      </Section>
      <div class={styles.actions}>
        {error && <Notice tone="error">{error}</Notice>}
        {message && <Notice tone="success">{message}</Notice>}
        <Button accent type="submit" disabled={busy}>{busy ? 'Saving…' : 'Save changes'}</Button>
      </div>
    </form>
  )
}

