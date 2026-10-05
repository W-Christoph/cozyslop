import type { ComponentChildren } from 'preact'
import type { Permission, RoomInfo } from '../../api'
import { Checkbox, Field, Input, Select } from '../ui/Field'
import { BanDate } from './BanDate'
import formStyles from '../ui/Form.module.css'
import styles from './PermissionFields.module.css'

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
  const cell = (label: string, children: ComponentChildren) => creating
    ? <Field label={label}>{children}</Field>
    : <td data-label={label}>{children}</td>
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
  const ban = () => <div class={styles.ban}>
    {checkbox('banned')}
    {/* A date only counts with the ban: picking one turns the ban on. */}
    <BanDate banned={draft.banned} until={until} busy={busy} name={name}
      onUntil={(value) => { onUntil(value); if (value && !draft.banned) onChange({ banned: true }) }} />
  </div>
  return (
    <>
      {!room && cell('Room', <>
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
      </>)}
      {cell('User', <>
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
      </>)}
      {cell('Remote', checkbox('remote'))}
      {cell('Images', checkbox('image'))}
      {cell('Upload', checkbox('upload'))}
      {cell('Trusted', checkbox('trusted'))}
      {cell('Invited', checkbox('invited'))}
      {cell('Invite name',
        <Input
          compact
          quiet class={styles.inviteName}
          placeholder="—"
          aria-label={`Invite name for ${name}`}
          maxLength={64}
          value={draft.inviteName}
          disabled={busy}
          onInput={(e) => onChange({ inviteName: e.currentTarget.value })}
        />
      )}
      {creating ? <div class={styles.banField}><span class={formStyles.label}>Banned</span>{ban()}</div> : <td data-label="Banned">{ban()}</td>}
    </>
  )
}
