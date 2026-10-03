import { useState } from 'preact/hooks'
import { Button } from '../../Button'
import { Modal } from '../../Modal'
import { useRoomStore } from '../RoomContext'
import styles from './WhisperModal.module.css'

export function WhisperModal({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const [target, setTarget] = useState('')
  const [body, setBody] = useState('')
  const [error, setError] = useState('')
  const users = [...store.users.value.values()].filter((user) => user.key !== store.selfKey.value)
  const selected = users.some((user) => user.key === target)
  const connected = store.server.value === 'connected'
  function send() {
    if (!store.rights.value.admin) return
    if (!selected) { setError('Please select a user who is still in the room.'); return }
    if (!connected) { setError('Wait for the room to reconnect before sending.'); return }
    if (!body.trim()) return
    store.whisper(target, body)
    setBody('')
    setError('')
  }
  if (!store.rights.value.admin) return null
  return (
    <Modal title="Whisper User" onClose={onClose}>
      <form class={styles.form} onSubmit={(e) => { e.preventDefault(); send() }}>
        <label class={styles.row}>User
          <select value={selected ? target : ''} onChange={(e) => { setTarget(e.currentTarget.value); setError('') }}>
            <option value="" disabled>Select User</option>
            {users.map((user) => <option key={user.key} value={user.key}>
              {user.nickname} ({user.username || `Anon ${user.key.slice(2, 6)}`})
            </option>)}
          </select>
        </label>
        <label class={styles.row}>Message
          <input type="text" value={body} maxLength={4096} onInput={(e) => { setBody(e.currentTarget.value); setError('') }} />
        </label>
        <Button type="submit" disabled={!selected || !body.trim() || !connected}>Send</Button>
        {!users.length && <p role="status">No other users are in the room.</p>}
        {!connected && <p role="status">Wait for the room to reconnect before sending.</p>}
        {error && <p role="alert">{error}</p>}
      </form>
    </Modal>
  )
}
