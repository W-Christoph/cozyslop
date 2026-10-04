import { useEffect, useMemo, useState } from 'preact/hooks'
import { api, type AnonGrant, type Permission } from '../../../api'
import type { User } from '../../../room/protocol'
import { blankPermission } from '../../admin/PermissionRow'
import { Button } from '../../Button'
import { useRoomStore } from '../RoomContext'
import { UserAvatar } from '../UserAvatar'
import styles from './CurrentRoomUserRow.module.css'

const checkboxes = [
  ['trusted', 'Trusted'], ['invited', 'Invited'],
  ['remote', 'Remote'], ['image', 'Images'], ['upload', 'Upload'],
] as const

export function CurrentRoomUserRow({ user, permission, grant, ready, onSaved, onGranted, onBan }: {
  user: User
  permission?: Permission
  grant?: AnonGrant // anonymous users: what an admin gave them
  ready: boolean
  onSaved: (permission: Permission) => void
  onGranted: (grant: AnonGrant) => void
  onBan: () => void
}) {
  const store = useRoomStore()
  const initial = useMemo(() => permission ?? {
    ...blankPermission(store.room), username: user.username,
  }, [permission, store.room, user.username])
  const [draft, setDraft] = useState(initial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  useEffect(() => {
    setDraft(initial)
  }, [initial])
  const [given, setGiven] = useState({ remote: grant?.remote ?? false, upload: grant?.upload ?? false })
  useEffect(() => {
    setGiven({ remote: grant?.remote ?? false, upload: grant?.upload ?? false })
  }, [grant?.remote, grant?.upload])

  function change(update: Partial<Permission>) {
    setDraft((current) => ({ ...current, ...update }))
    setError('')
    setMessage('')
  }
  async function save() {
    if (busy || !ready || user.anonymous || !store.rights.value.admin) return
    setBusy(true)
    setError('')
    setMessage('')
    // Bans are not editable here; preserve the values loaded from the API.
    const { remote, image, upload, trusted, invited, inviteName, banned, bannedUntil } = draft
    try {
      const saved = await api.put<Permission>(
        `/api/admin/permissions/${encodeURIComponent(store.room)}/${encodeURIComponent(user.username)}`,
        { remote, image, upload, trusted, invited, inviteName, banned, bannedUntil },
      )
      onSaved(saved)
      setMessage('Permission saved!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  // Anonymous users have no account to save a permission on: what they are
  // given lasts while they are in the room.
  async function give() {
    if (busy || !ready || !user.anonymous || !store.rights.value.admin) return
    setBusy(true)
    setError('')
    setMessage('')
    try {
      onGranted(await api.put<AnonGrant>(`/api/admin/rooms/${encodeURIComponent(store.room)}/grants`, { key: user.key, ...given }))
      setMessage('Permission saved!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  function givenCheckbox(field: 'remote' | 'upload', label: string) {
    return <td>
      <input type="checkbox" checked={given[field]} disabled={busy || !ready}
        aria-label={`${label} for anonymous user ${user.nickname}`}
        onChange={(e) => { setGiven((current) => ({ ...current, [field]: e.currentTarget.checked })); setError(''); setMessage('') }} />
    </td>
  }
  function checkbox(index: number) {
    const [field, label] = checkboxes[index]
    return <td key={field}>
      <input type="checkbox" checked={draft[field]} disabled={busy || !ready}
        aria-label={`${label} for ${user.username}`}
        onChange={(e) => change({ [field]: e.currentTarget.checked })} />
    </td>
  }
  return (
    <tr>
      <td><UserAvatar user={user} /></td>
      <td class={styles.identity}>
        <span style={{ color: user.nameColor }}>{user.nickname}</span>
        {user.admin && <span class={styles.badge}>Admin</span>}
      </td>
      <td title={user.key}>{user.anonymous ? `Anonymous (${user.key.slice(2, 6)})` : user.username}</td>
      {user.anonymous ? <>
        <td colSpan={3} class={styles.note}>No account: lasts until they leave the room or reload.</td>
        {givenCheckbox('remote', 'Remote')}
        <td />
        {givenCheckbox('upload', 'Upload')}
        <td class={styles.actions}>
          <Button disabled={busy || !ready} onClick={() => { void give() }}>Save</Button>
          {error && <p role="alert">{error}</p>}
          {message && <p role="status">{message}</p>}
        </td>
      </> : <>
        {checkbox(0)}{checkbox(1)}
        <td><input class={styles.inviteName} aria-label={`Invite name for ${user.username}`}
          value={draft.inviteName} maxLength={64} disabled={busy || !ready}
          onInput={(e) => change({ inviteName: e.currentTarget.value })} /></td>
        {checkbox(2)}{checkbox(3)}{checkbox(4)}
        <td class={styles.actions}>
          <Button disabled={busy || !ready} onClick={() => { void save() }}>Save</Button>
          {error && <p role="alert">{error}</p>}
          {message && <p role="status">{message}</p>}
        </td>
      </>}
      <td><Button disabled={busy} onClick={onBan}>Ban/Kick</Button></td>
    </tr>
  )
}
