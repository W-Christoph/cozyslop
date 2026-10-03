import type { Permission, RoomInfo } from '../../api'

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
    <input
      type="checkbox"
      aria-label={`${flag} for ${draft.username || 'new permission'}`}
      disabled={busy}
      checked={draft[flag]}
      onChange={(e) => onChange({ [flag]: e.currentTarget.checked })}
    />
  )
  return (
    <>
      <td>
        {creating && !room ? (
          <select
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
          </select>
        ) : (
          draft.room
        )}
      </td>
      <td>
        {creating ? (
          <input
            aria-label="Username for new permission"
            maxLength={12}
            value={draft.username}
            disabled={busy}
            onInput={(e) => onChange({ username: e.currentTarget.value })}
          />
        ) : (
          draft.username
        )}
      </td>
      <td>{checkbox('banned')}</td>
      <td>{checkbox('trusted')}</td>
      <td>{checkbox('invited')}</td>
      <td>
        <input
          aria-label={`Invite name for ${draft.username || 'new permission'}`}
          maxLength={64}
          value={draft.inviteName}
          disabled={busy}
          onInput={(e) => onChange({ inviteName: e.currentTarget.value })}
        />
      </td>
      <td>{checkbox('remote')}</td>
      <td>{checkbox('image')}</td>
      <td>{checkbox('upload')}</td>
      <td>
        <input
          type="datetime-local"
          aria-label={`Banned until for ${draft.username || 'new permission'} (empty means forever)`}
          title="Empty = forever when banned"
          value={until}
          disabled={busy || !draft.banned}
          onInput={(e) => onUntil(e.currentTarget.value)}
        />
      </td>
    </>
  )
}
