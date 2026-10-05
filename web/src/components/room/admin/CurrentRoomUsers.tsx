import { useEffect, useState } from 'preact/hooks'
import { api, type AnonGrant, type Permission } from '../../../api'
import type { User } from '../../../room/protocol'
import { AdminTable } from '../../admin/AdminTable'
import { Button } from '../../Button'
import { Spinner } from '../../ui/Spinner'
import { Notice } from '../../ui/Notice'
import { useRoomStore } from '../RoomContext'
import { BanModal } from './BanModal'
import { CurrentRoomUserRow } from './CurrentRoomUserRow'
import { RoomDefaultsRow } from './RoomDefaultsRow'
import styles from './CurrentRoomUsers.module.css'

export function CurrentRoomUsers() {
  const store = useRoomStore()
  const [permissions, setPermissions] = useState<Permission[]>([])
  const [grants, setGrants] = useState<AnonGrant[]>([])
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
    void Promise.all([
      api.get<Permission[]>(`/api/admin/permissions?room=${encodeURIComponent(store.room)}`),
      api.get<AnonGrant[]>(`/api/admin/rooms/${encodeURIComponent(store.room)}/grants`),
    ])
      .then(([permissions, grants]) => { if (active) { setPermissions(permissions); setGrants(grants) } })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Something went wrong.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [store.room, revision, admin])

  // What an anonymous user was given ends when they leave the room; someone
  // who comes back starts without it.
  const present = store.users.value
  useEffect(() => {
    setGrants((current) => current.every((grant) => present.has(grant.key)) ? current : current.filter((grant) => present.has(grant.key)))
  }, [present])

  if (!admin) return null
  const users = [...store.users.value.values()]
  const byUsername = new Map(permissions.map((permission) => [permission.username, permission]))
  const byKey = new Map(grants.map((grant) => [grant.key, grant]))
  return (
    <>
      {error && <Notice tone="error">{error}</Notice>}
      <div class={styles.toolbar}>
        {loading ? <Spinner inline label="Loading permissions…" /> : <span>{users.length === 1 ? '1 person' : `${users.length} people`} in the room</span>}
        <Button size="sm" variant="ghost" icon="refresh" disabled={loading} onClick={() => setRevision((value) => value + 1)}>Refresh</Button>
      </div>
      <AdminTable headings={['User', 'Trusted', 'Invited', 'Invite name', 'Remote', 'Images', 'Upload', '']}>
        <RoomDefaultsRow />
        {users.map((user) => (
          <CurrentRoomUserRow key={user.key} user={user} permission={byUsername.get(user.username)}
            grant={user.anonymous ? byKey.get(user.key) : undefined}
            ready={!loading && !error} onBan={() => setTarget(user)}
            onGranted={(saved) => setGrants((current) => [...current.filter((row) => row.key !== saved.key), saved])}
            onSaved={(saved) => setPermissions((current) => [...current.filter((row) => row.username !== saved.username), saved])} />
        ))}
      </AdminTable>
      {target && <BanModal user={target} onClose={() => setTarget(null)} onBanned={() => {
        setTarget(null)
        setRevision((value) => value + 1)
      }} />}
    </>
  )
}
