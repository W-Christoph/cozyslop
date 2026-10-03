import { Button } from '../../Button'
import { useRoomSettingsForm } from './useRoomSettingsForm'

const fields = ['defaultRemote', 'defaultImage', 'defaultUpload'] as const
const labels = ['Default remote permission', 'Default image permission', 'Default upload permission']

export function RoomDefaultsRow() {
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  return (
    <tr>
      <td colSpan={6}>Room defaults</td>
      {fields.map((field, index) => (
        <td key={field}>
          <input type="checkbox" aria-label={labels[index]} checked={draft?.[field] ?? false}
            disabled={busy || !draft} onChange={(e) => change(field, e.currentTarget.checked)} />
        </td>
      ))}
      <td colSpan={2}>
        <Button disabled={busy || !draft} onClick={() => { void save() }}>Update defaults</Button>
        {!draft && <p role="status">Loading room settings...</p>}
        {error && <p role="alert">{error}</p>}
        {message && <p role="status">{message}</p>}
      </td>
    </tr>
  )
}
