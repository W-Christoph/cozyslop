import { Button } from '../../Button'
import { Checkbox } from '../../ui/Field'
import { useRoomStore } from '../RoomContext'
import { useRoomSettingsForm } from './useRoomSettingsForm'
import styles from './CurrentRoomUserRow.module.css'

const fields = ['defaultRemote', 'defaultImage', 'defaultUpload'] as const
const labels = ['Default remote permission', 'Default image permission', 'Default upload permission']

export function RoomDefaultsRow() {
  const store = useRoomStore()
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  const saved = store.settings.value
  const dirty = !!draft && !!saved && fields.some((field) => draft[field] !== saved[field])
  return (
    <tr class={styles.defaults}>
      <td colSpan={4} class={styles.defaultsLabel}>Room defaults<span>For everyone without a permission of their own.</span></td>
      {fields.map((field, index) => (
        <td key={field} data-label={labels[index]}>
          <Checkbox aria-label={labels[index]} checked={draft?.[field] ?? false}
            disabled={busy || !draft} onChange={(e) => change(field, e.currentTarget.checked)} />
        </td>
      ))}
      <td>
        <div class={styles.actions}>
          <Button size="sm" variant={dirty ? 'primary' : 'ghost'} disabled={busy || !draft || !dirty} onClick={() => { void save() }} aria-label="Update defaults">Save</Button>
          <span />
        </div>
        {!draft && <p class={styles.status} role="status">Loading room settings...</p>}
        {error && <p class={styles.error} role="alert">{error}</p>}
        {message && <p class={styles.status} role="status">{message}</p>}
      </td>
    </tr>
  )
}
