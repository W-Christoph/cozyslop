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
      setStatus(creating ? 'Permission created!' : 'Permission saved!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
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
    <tr>
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
      <td class={styles.actions}>
        <Button accent disabled={busy} onClick={save}>
          {creating ? 'Create' : 'Save'}
        </Button>
        {error && <p role="alert">{error}</p>}
        {status && <p role="status">{status}</p>}
      </td>
      <td>
        <Button
          accent
          disabled={busy}
          onClick={
            creating
              ? () => {
                  setDraft(blankPermission(room ?? rooms[0]?.name))
                  setUntil('')
                  setError('')
                  setStatus('')
                }
              : remove
          }
        >
          {creating ? 'Clear' : 'Delete'}
        </Button>
      </td>
    </tr>
  )
}
