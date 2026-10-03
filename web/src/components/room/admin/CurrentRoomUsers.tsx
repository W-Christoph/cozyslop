import { useEffect, useState } from 'preact/hooks'
import { api, type Permission } from '../../../api'
import type { User } from '../../../room/protocol'
import { AdminTable } from '../../admin/AdminTable'
import { Button } from '../../Button'
import { useRoomStore } from '../RoomContext'
import { BanModal } from './BanModal'
import { CurrentRoomUserRow } from './CurrentRoomUserRow'
import { RoomDefaultsRow } from './RoomDefaultsRow'

export function CurrentRoomUsers() {
  const store = useRoomStore()
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [target, setTarget] = useState<User | null>(null)
  const admin = store.rights.value.admin
  useEffect(() => {
    if (!admin) return
    let active = true
    setLoading(true)
    setError('')
    void api.get<Permission[]>(`/api/admin/permissions?room=${encodeURIComponent(store.room)}`)
      .then((result) => { if (active) setPermissions(result) })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Something went wrong.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [store.room, revision, admin])

  if (!admin) return null
  const users = [...store.users.value.values()]
  const byUsername = new Map(permissions.map((permission) => [permission.username, permission]))
  return (
    <>
      <Button disabled={loading} onClick={() => setRevision((value) => value + 1)}>Refresh permissions</Button>
      {loading && <p role="status">Loading permissions...</p>}
      {error && <p role="alert">{error}</p>}
      <AdminTable headings={['Avatar', 'Nickname', 'User', 'Trusted', 'Invited', 'Invite Name', 'Remote', 'Images', 'Upload', '', '']}>
        <RoomDefaultsRow />
        {users.map((user) => (
          <CurrentRoomUserRow key={user.key} user={user} permission={byUsername.get(user.username)}
            ready={!loading && !error} onBan={() => setTarget(user)}
            onSaved={(saved) => setPermissions((current) => [...current.filter((row) => row.username !== saved.username), saved])} />
        ))}
      </AdminTable>
      {!users.length && <p>No users are currently in the room.</p>}
      {target && <BanModal user={target} onClose={() => setTarget(null)} onBanned={() => {
        setTarget(null)
        setRevision((value) => value + 1)
      }} />}
    </>
  )
}
