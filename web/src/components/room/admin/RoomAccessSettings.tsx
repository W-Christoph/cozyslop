import { Button } from '../../Button'
import type { Access, RoomSettings } from '../../../room/protocol'
import { useRoomSettingsForm } from './useRoomSettingsForm'
import styles from './RoomAccessSettings.module.css'

const accessOptions: [Access, string][] = [
  ['public', 'Public'], ['account', 'Users'],
  ['verified', 'Verified Users'], ['invite', 'Invited Users'],
]
const checkboxes = [
  ['hidden', 'Hidden To Unauthorized'],
  ['defaultImage', 'Default Image Permission'],
  ['defaultRemote', 'Default Remote Permission'],
  ['defaultUpload', 'Default Upload Permission'],
  ['remoteOwnership', 'Remote Ownership'],
  ['centerRemote', 'Always Center Remote'],
] as const satisfies readonly (readonly [keyof RoomSettings, string])[]
const fields = ['access', ...checkboxes.map(([key]) => key)] as const

export function RoomAccessSettings() {
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  return (
    <form class={styles.form} onSubmit={(e) => { e.preventDefault(); void save() }}>
      <h2 class={styles.heading}>Room Access</h2>
      {!draft ? <p role="status">Loading room settings...</p> : (
        <fieldset disabled={busy} class={styles.fields}>
          <legend>Access</legend>
          {accessOptions.map(([value, label]) => (
            <label key={value} class={styles.choice}>
              <input type="radio" name="roomAccess" value={value} checked={draft.access === value}
                onChange={() => change('access', value)} />{label}
            </label>
          ))}
          {checkboxes.map(([key, label]) => (
            <label key={key} class={styles.choice}>
              <input type="checkbox" checked={draft[key]} onChange={(e) => change(key, e.currentTarget.checked)} />{label}
            </label>
          ))}
        </fieldset>
      )}
      <Button type="submit" disabled={busy || !draft}>Update Room Access</Button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </form>
  )
}
