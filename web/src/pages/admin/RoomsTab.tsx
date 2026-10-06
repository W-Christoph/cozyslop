import { useEffect, useState } from 'preact/hooks'
import { adminRooms, type AdminRoom } from '../../api'
import { Button } from '../../components/Button'
import { AdminTable } from '../../components/admin/AdminTable'
import { RoomModal, type RoomAction } from '../../components/admin/RoomModal'
import { Badge } from '../../components/ui/Badge'
import { EmptyState } from '../../components/ui/EmptyState'
import { Notice } from '../../components/ui/Notice'
import { Section } from '../../components/ui/Section'
import { Spinner } from '../../components/ui/Spinner'
import styles from './RoomsTab.module.css'

const REFRESH_MS = 10_000

function Connection({ room }: { room: AdminRoom }) {
  if (room.connected) return <Badge tone="success">Online</Badge>
  if (!room.offlineSince) return <Badge>Connecting</Badge>
  return <>
    <Badge tone="warning">Offline</Badge>{' '}
    <span class={styles.note}>since {new Date(room.offlineSince).toLocaleString()}</span>
  </>
}

export function RoomsTab() {
  const [rooms, setRooms] = useState<AdminRoom[]>([]),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [loading, setLoading] = useState(true),
    [refresh, setRefresh] = useState(0),
    [action, setAction] = useState<RoomAction | null>(null)
  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    setMessage('')
    void adminRooms.list().then((list) => {
      if (active) setRooms(list)
    }).catch((e) => {
      if (active) setError(e instanceof Error ? e.message : 'Something went wrong.')
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [refresh])
  // Keep connection states current without the loading spinner.
  useEffect(() => {
    const timer = window.setInterval(() => {
      void adminRooms.list().then(setRooms).catch(() => {})
    }, REFRESH_MS)
    return () => window.clearInterval(timer)
  }, [])
  function saved(room: AdminRoom) {
    setRooms((list) => [...list.filter((r) => r.name !== room.name), room].sort((a, b) => a.name.localeCompare(b.name)))
    setMessage(`${room.name} ${action?.mode === 'add' ? 'added' : 'updated'}.`)
  }
  return (
    <Section title="Rooms" description="Manage room connections. Adding a room registers its address; start its container separately."
      actions={<>
        <Button icon="refresh" disabled={loading} onClick={() => setRefresh((value) => value + 1)}>Refresh</Button>
        <Button variant="primary" icon="plus" disabled={loading} onClick={() => setAction({ mode: 'add' })}>Add room</Button>
      </>}>
      {loading && <Spinner label="Loading rooms…" />}
      {error && <Notice tone="error">{error}</Notice>}
      {message && <Notice tone="success">{message}</Notice>}
      {!loading && !error && rooms.length === 0 && <EmptyState icon="monitor" title="No rooms">Add a room to connect a neko container.</EmptyState>}
      {!loading && rooms.length > 0 && <AdminTable headings={['Room', 'Source', 'Connection', 'People', 'Actions']}>
        {rooms.map((room) => <tr key={room.name}>
          <td><a class={styles.name} href={`/room/${encodeURIComponent(room.name)}`}>{room.name}</a></td>
          <td data-label="Source"><Badge>{room.source === 'configured' ? 'Configured' : 'Registered'}</Badge></td>
          <td data-label="Connection"><Connection room={room} /></td>
          <td data-label="People">{room.userCount}</td>
          <td>{room.source === 'configured' ? <span class={styles.note}>Defined in <code>COZYCAST_ROOMS</code>.</span> : <div class={styles.actions}>
            <Button size="sm" variant="ghost" onClick={() => setAction({ mode: 'address', room })} aria-label={`Change address of ${room.name}`}>Change address</Button>
            <Button size="sm" variant="ghost" onClick={() => setAction({ mode: 'token', room })} aria-label={`New token for ${room.name}`}>New token</Button>
            <Button size="sm" variant="danger-ghost" onClick={() => setAction({ mode: 'remove', room })} aria-label={`Remove ${room.name}`}>Remove</Button>
          </div>}</td>
        </tr>)}
      </AdminTable>}
      {action && <RoomModal key={`${action.mode}:${action.mode === 'add' ? '' : action.room.name}`} action={action}
        onClose={() => setAction(null)} onSaved={saved} onRemoved={(name) => {
          setRooms((list) => list.filter((room) => room.name !== name))
          setMessage(`${name} removed.`)
        }} />}
    </Section>
  )
}
