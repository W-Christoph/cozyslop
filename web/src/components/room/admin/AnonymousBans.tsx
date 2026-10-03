import { useEffect, useState } from 'preact/hooks'
import { api } from '../../../api'
import { AdminTable } from '../../admin/AdminTable'
import { Button } from '../../Button'
import { useRoomStore } from '../RoomContext'

interface AnonymousBan {
  id: number
  room: string
  ip: string
  bannedUntil: number | null
}

export function AnonymousBans() {
  const store = useRoomStore()
  const [bans, setBans] = useState<AnonymousBan[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const [revision, setRevision] = useState(0)
  const admin = store.rights.value.admin
  useEffect(() => {
    if (!admin) return
    let active = true
    setLoading(true)
    setError('')
    setMessage('')
    void api.get<AnonymousBan[]>(`/api/admin/bans?room=${encodeURIComponent(store.room)}`)
      .then((result) => { if (active) setBans(result) })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Something went wrong.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [store.room, revision, admin])

  async function unban(id: number) {
    if (busy || !store.rights.value.admin) return
    setBusy(true)
    setError('')
    setMessage('')
    try {
      await api.del(`/api/admin/bans/${id}`)
      setBans((current) => current.filter((ban) => ban.id !== id))
      setMessage('Ban removed!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  if (!admin) return null
  return (
    <>
      <Button disabled={busy || loading} onClick={() => setRevision((value) => value + 1)}>Refresh bans</Button>
      {loading && <p role="status">Loading anonymous bans...</p>}
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      {!loading && !error && !bans.length && <p>No active anonymous bans.</p>}
      {!loading && bans.length > 0 && <AdminTable headings={['IP', 'Banned until', '']}>
        {bans.map((ban) => <tr key={ban.id}>
          <td>{ban.ip}</td>
          <td>{ban.bannedUntil === null ? 'forever' : new Date(ban.bannedUntil * 1000).toLocaleString()}</td>
          <td><Button disabled={busy} aria-label={`Unban ${ban.ip}`} onClick={() => { void unban(ban.id) }}>Unban</Button></td>
        </tr>)}
      </AdminTable>}
    </>
  )
}
