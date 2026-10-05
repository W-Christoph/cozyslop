import { useState } from 'preact/hooks'
import { Button } from '../../Button'
import { Field, Input, Select } from '../../ui/Field'
import { Notice } from '../../ui/Notice'
import { useRoomStore } from '../RoomContext'
import { userIdentity } from '../UserHoverName'
import styles from './Whisper.module.css'

export function Whisper() {
  const store = useRoomStore()
  const [target, setTarget] = useState('')
  const [body, setBody] = useState('')
  const [error, setError] = useState('')
  const [sent, setSent] = useState('')
  const users = [...store.users.value.values()].filter((user) => user.key !== store.selfKey.value)
  const selected = users.find((user) => user.key === target)
  const connected = store.server.value === 'connected'
  function send() {
    if (!store.rights.value.admin) return
    if (!selected) { setError('Please select a user who is still in the room.'); return }
    if (!connected) { setError('Wait for the room to reconnect before sending.'); return }
    if (!body.trim()) return
    store.whisper(target, body)
    setBody('')
    setError('')
    setSent(`Whispered to ${selected.nickname}.`)
  }
  if (!store.rights.value.admin) return null
  return (
    <form class={styles.form} onSubmit={(e) => { e.preventDefault(); send() }}>
      <Field label="To" class={styles.target}>
        <Select value={selected ? target : ''} onChange={(e) => { setTarget(e.currentTarget.value); setError(''); setSent('') }}>
          <option value="" disabled>Select user</option>
          {users.map((user) => <option key={user.key} value={user.key}>
            {user.nickname} ({userIdentity(user)})
          </option>)}
        </Select>
      </Field>
      <Field label="Message" class={styles.message}>
        <Input type="text" value={body} maxLength={4096} onInput={(e) => { setBody(e.currentTarget.value); setError(''); setSent('') }} />
      </Field>
      <Button variant="primary" type="submit" disabled={!selected || !body.trim() || !connected}>Send</Button>
      <div class={styles.notes}>
        {!users.length && <Notice>No other users are in the room.</Notice>}
        {!connected && <Notice>Wait for the room to reconnect before sending.</Notice>}
        {error && <Notice tone="error">{error}</Notice>}
        {sent && <Notice tone="success">{sent}</Notice>}
      </div>
    </form>
  )
}
