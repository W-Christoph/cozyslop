import { useState } from 'preact/hooks'
import { api } from '../../../api'
import type { User } from '../../../room/protocol'
import { Button } from '../../Button'
import { Modal } from '../../Modal'
import { useRoomStore } from '../RoomContext'
import styles from './BanModal.module.css'

const durations = [
  ['0', 'Refresh (kick)'], ['10', '10 minutes'], ['60', '1 hour'],
  ['1440', '1 day'], ['10080', '1 week'], ['43200', '1 month'], ['', 'Unlimited'],
] as const

export function BanModal({ user, onClose, onBanned }: {
  user: User
  onClose: () => void
  onBanned: () => void
}) {
  const store = useRoomStore()
  const [duration, setDuration] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const present = store.users.value.has(user.key)
  async function ban() {
    if (busy || !present || !store.rights.value.admin) return
    setBusy(true)
    setError('')
    try {
      await api.post(`/api/admin/rooms/${encodeURIComponent(store.room)}/bans`, {
        key: user.key, minutes: duration === '' ? null : Number(duration),
      })
      onBanned()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  if (!store.rights.value.admin) return null
  return (
    <Modal title="Ban/Kick" onClose={onClose} compact>
      <form class={styles.form} onSubmit={(e) => { e.preventDefault(); void ban() }}>
        <p>User: {user.nickname} ({user.username || `Anon ${user.key.slice(2, 6)}`})</p>
        <label class={styles.row}>Expiration
          <select value={duration} disabled={busy} onChange={(e) => { setDuration(e.currentTarget.value); setError('') }}>
            {durations.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </select>
        </label>
        <Button type="submit" disabled={busy || !present}>{duration === '0' ? 'Kick User' : 'Ban User'}</Button>
        {!present && !busy && <p role="status">That user is no longer in the room.</p>}
        {error && <p role="alert">{error}</p>}
      </form>
    </Modal>
  )
}
