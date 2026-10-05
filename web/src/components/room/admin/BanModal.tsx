import { useId, useState } from 'preact/hooks'
import { api } from '../../../api'
import type { User } from '../../../room/protocol'
import { Button } from '../../Button'
import { Modal } from '../../Modal'
import { Avatar } from '../../ui/Avatar'
import { Field, Select } from '../../ui/Field'
import { Notice } from '../../ui/Notice'
import { useRoomStore } from '../RoomContext'
import { userIdentity } from '../UserHoverName'
import formStyles from '../../ui/Form.module.css'
import styles from './BanModal.module.css'

const durations = [
  ['0', 'Not at all (kick)'], ['10', '10 minutes'], ['60', '1 hour'],
  ['1440', '1 day'], ['10080', '1 week'], ['43200', '1 month'], ['', 'Forever'],
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
  const form = useId()
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
  const kick = duration === '0'
  return (
    <Modal title={`${kick ? 'Kick' : 'Ban'} ${user.nickname}`} onClose={onClose} size="sm" footer={<>
      <Button onClick={onClose}>Cancel</Button>
      <Button variant="danger" type="submit" form={form} disabled={busy || !present}>{kick ? 'Kick user' : 'Ban user'}</Button>
    </>}>
      <form id={form} class={formStyles.form} onSubmit={(e) => { e.preventDefault(); void ban() }}>
        <div class={styles.user}>
          <Avatar src={user.avatarUrl} size={32} />
          <div>
            <strong>{user.nickname}</strong>
            <span>{userIdentity(user)}</span>
          </div>
        </div>
        <Field label="Keep them out for" hint={kick ? 'They are removed from the room and can come straight back.' : user.anonymous ? 'Bans their address; other anonymous visitors on it are removed too.' : undefined}>
          <Select value={duration} disabled={busy} onChange={(e) => { setDuration(e.currentTarget.value); setError('') }}>
            {durations.map(([value, label]) => <option key={value} value={value}>{label}</option>)}
          </Select>
        </Field>
        {!present && !busy && <Notice>That user is no longer in the room.</Notice>}
        {error && <Notice tone="error">{error}</Notice>}
      </form>
    </Modal>
  )
}
