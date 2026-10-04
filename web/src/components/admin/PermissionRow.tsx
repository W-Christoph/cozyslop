import { useEffect, useState } from 'preact/hooks'
import { api, type Permission, type RoomInfo } from '../../api'
import { Button } from '../Button'
import { PermissionFields } from './PermissionFields'
import styles from './PermissionRow.module.css'

export function blankPermission(room = ''): Permission {
  return {
    room,
    username: '',
    remote: false,
    image: false,
    upload: false,
    trusted: false,
    invited: false,
    banned: false,
    inviteName: '',
    bannedUntil: null,
  }
}
function datetime(timestamp: number | null) {
  if (timestamp === null) return ''
  const date = new Date(timestamp * 1000)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16)
}

export function PermissionRow({
  permission,
  room,
  rooms,
  creating = false,
  onSaved,
  onDeleted,
}: {
  permission: Permission
  room?: string
  rooms: RoomInfo[]
  creating?: boolean
  onSaved: (permission: Permission) => void
  onDeleted: () => void
}) {
  const [draft, setDraft] = useState(permission),
    [until, setUntil] = useState(datetime(permission.bannedUntil))
  const [error, setError] = useState(''),
    [status, setStatus] = useState(''),
    [busy, setBusy] = useState(false)
  useEffect(() => {
    setDraft(permission)
    setUntil(datetime(permission.bannedUntil))
  }, [permission])
  function change(update: Partial<Permission>) {
    setDraft((value) => ({ ...value, ...update }))
    setStatus('')
  }
  async function save() {
    setError('')
    setStatus('')
    if (!draft.room || !draft.username.trim()) {
      setError('Please choose a room and enter a username.')
      return
    }
    const bannedUntil =
      draft.banned && until
        ? Math.floor(new Date(until).getTime() / 1000)
        : null
    if (bannedUntil !== null && !Number.isFinite(bannedUntil)) {
      setError('Please enter a valid ban date and time.')
      return
    }
    const { remote, image, upload, trusted, invited, banned, inviteName } =
      draft
    setBusy(true)
    try {
      const result = await api.put<Permission>(
        `/api/admin/permissions/${encodeURIComponent(draft.room)}/${encodeURIComponent(draft.username.trim())}`,
        {
          remote,
          image,
          upload,
          trusted,
          invited,
          banned,
          inviteName,
          bannedUntil,
        },
      )
      onSaved(result)
      setDraft(creating ? blankPermission(room ?? rooms[0]?.name) : result)
      setUntil(creating ? '' : datetime(result.bannedUntil))
      setStatus(creating ? 'Permission added.' : 'Saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  const dirty = (creating && !!draft.username.trim()) || until !== datetime(permission.bannedUntil) ||
    (Object.keys(draft) as (keyof Permission)[]).some((key) => key !== 'bannedUntil' && draft[key] !== permission[key])
  async function remove() {
    setBusy(true)
    setError('')
    setStatus('')
    try {
      await api.del(
        `/api/admin/permissions/${encodeURIComponent(draft.room)}/${encodeURIComponent(draft.username)}`,
      )
      onDeleted()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <tr class={creating ? styles.creating : undefined}>
      <PermissionFields
        draft={draft}
        room={room}
        rooms={rooms}
        creating={creating}
        busy={busy}
        until={until}
        onChange={change}
        onUntil={(value) => {
          setUntil(value)
          setStatus('')
        }}
      />
      <td>
        <div class={styles.actions}>
          <Button size="sm" variant={dirty ? 'primary' : 'ghost'} disabled={busy || !dirty} onClick={save}>
            {creating ? 'Add' : 'Save'}
          </Button>
          {creating ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onClick={() => {
                setDraft(blankPermission(room ?? rooms[0]?.name))
                setUntil('')
                setError('')
                setStatus('')
              }}
            >
              Clear
            </Button>
          ) : (
            <Button
              size="sm"
              variant="ghost"
              icon="trash"
              class={styles.delete}
              aria-label={`Delete the permission of ${draft.username}`}
              title="Delete"
              disabled={busy}
              onClick={remove}
            />
          )}
        </div>
        {error && <p class={styles.error} role="alert">{error}</p>}
        {status && <p class={styles.status} role="status">{status}</p>}
      </td>
    </tr>
  )
}
