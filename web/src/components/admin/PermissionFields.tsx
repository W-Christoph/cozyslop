import type { Permission, RoomInfo } from '../../api'
import { Checkbox, Input, Select } from '../ui/Field'
import styles from './PermissionRow.module.css'

export function PermissionFields({
  draft,
  room,
  rooms,
  creating,
  busy,
  until,
  onChange,
  onUntil,
}: {
  draft: Permission
  room?: string
  rooms: RoomInfo[]
  creating: boolean
  busy: boolean
  until: string
  onChange: (update: Partial<Permission>) => void
  onUntil: (value: string) => void
}) {
  const checkbox = (
    flag: 'banned' | 'trusted' | 'invited' | 'remote' | 'image' | 'upload',
  ) => (
    <Checkbox
      aria-label={`${flag} for ${draft.username || 'new permission'}`}
      disabled={busy}
      checked={draft[flag]}
      onChange={(e) => onChange({ [flag]: e.currentTarget.checked })}
    />
  )
  const name = draft.username || 'new permission'
  return (
    <>
      {!room && <td>
        {creating ? (
          <Select
            compact
            class={styles.room}
            aria-label="Room for new permission"
            value={draft.room}
            disabled={busy}
            onChange={(e) => onChange({ room: e.currentTarget.value })}
          >
            <option value="">Select room</option>
            {rooms.map((value) => (
              <option key={value.name} value={value.name}>
                {value.name}
              </option>
            ))}
          </Select>
        ) : (
          draft.room
        )}
      </td>}
      <td>
        {creating ? (
          <Input
            compact
            class={styles.username}
            placeholder="Username"
            aria-label="Username for new permission"
            maxLength={12}
            value={draft.username}
            disabled={busy}
            onInput={(e) => onChange({ username: e.currentTarget.value })}
          />
        ) : (
          <strong>{draft.username}</strong>
        )}
      </td>
      <td data-label="Remote">{checkbox('remote')}</td>
      <td data-label="Images">{checkbox('image')}</td>
      <td data-label="Upload">{checkbox('upload')}</td>
      <td data-label="Trusted">{checkbox('trusted')}</td>
      <td data-label="Invited">{checkbox('invited')}</td>
      <td data-label="Invite name">
        <Input
          compact
          class={`${styles.inviteName} ${styles.quiet}`}
          placeholder="—"
          aria-label={`Invite name for ${name}`}
          maxLength={64}
          value={draft.inviteName}
          disabled={busy}
          onInput={(e) => onChange({ inviteName: e.currentTarget.value })}
        />
      </td>
      <td data-label="Banned">
        <div class={styles.ban}>
          {checkbox('banned')}
          {draft.banned && (
            <Input
              compact
              class={styles.until}
              type="datetime-local"
              aria-label={`Banned until for ${name} (empty means forever)`}
              title="Until when. Empty = banned forever"
              value={until}
              disabled={busy}
              onInput={(e) => onUntil(e.currentTarget.value)}
            />
          )}
        </div>
      </td>
    </>
  )
}
