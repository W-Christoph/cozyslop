import { useId, useState } from 'preact/hooks'
import { adminRooms, type AdminRoom } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Field, Input, Textarea } from '../ui/Field'
import { Notice } from '../ui/Notice'
import { nekoUrlError, roomNameError } from './roomValidation'
import formStyles from '../ui/Form.module.css'
import styles from './RoomModal.module.css'

export type RoomAction = { mode: 'add' } | { mode: 'address' | 'token' | 'remove'; room: AdminRoom }

export function RoomModal({ action, onClose, onSaved, onRemoved }: {
  action: RoomAction
  onClose: () => void
  onSaved: (room: AdminRoom) => void
  onRemoved: (name: string) => void
}) {
  const { mode } = action
  const [name, setName] = useState(mode === 'add' ? '' : action.room.name),
    [nekoUrl, setNekoUrl] = useState('')
  const [token, setToken] = useState(''),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false)
  const form = useId()
  const editing = mode === 'add' || mode === 'address'
  const environment = `COZYCAST_ROOM=${name}\nCOZYCAST_NEKO_TOKEN=${token}`
  const close = () => { if (!busy) onClose() }
  async function save() {
    if (busy || token) return
    setError('')
    if (editing) {
      const invalid = (mode === 'add' ? roomNameError(name) : '') || nekoUrlError(nekoUrl)
      if (invalid) { setError(invalid); return }
    }
    setBusy(true)
    try {
      if (mode === 'remove') {
        await adminRooms.remove(name)
        onRemoved(name)
        onClose()
      } else {
        const room = mode === 'add' ? await adminRooms.create(name, nekoUrl)
          : mode === 'address' ? await adminRooms.changeAddress(name, nekoUrl)
          : await adminRooms.newToken(name)
        // Do not pass the issuance response (and its token) into table state.
        onSaved({ name: room.name, source: room.source, connected: room.connected, userCount: room.userCount,
          ...(room.offlineSince ? { offlineSince: room.offlineSince } : {}) })
        if ('nekoToken' in room && typeof room.nekoToken === 'string') setToken(room.nekoToken)
        else onClose()
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function copy(value: string, label: string) {
    setError('')
    setMessage('')
    try {
      await navigator.clipboard.writeText(value)
      setMessage(`${label} copied.`)
    } catch {
      setError('Could not copy. Select the text and copy it manually.')
    }
  }
  const title = token ? `Room token: ${name}` : mode === 'add' ? 'Add room'
    : mode === 'address' ? `Change address: ${name}` : mode === 'token' ? `New token: ${name}` : `Remove room: ${name}`
  const submit = mode === 'add' ? 'Add room' : mode === 'address' ? 'Save address' : mode === 'token' ? 'New token' : 'Remove'
  return (
    <Modal title={title} onClose={close} footer={<>
      <Button disabled={busy} onClick={close}>{token ? 'Done' : 'Cancel'}</Button>
      {!token && <Button variant={mode === 'remove' ? 'danger' : 'primary'} disabled={busy}
        type={editing ? 'submit' : 'button'} form={editing ? form : undefined}
        onClick={editing ? undefined : save}>{submit}</Button>}
    </>}>
      {token ? <div class={formStyles.form}>
        <Notice>The token will not be shown again; you can replace it with “New token”.</Notice>
        <p>{mode === 'token' ? 'Restart the room’s container with the new token and the environment below.'
          : 'Set the environment below on the room’s container, then start or restart it.'}</p>
        <div class={styles.copyRow}>
          <Field label="Token">
            <Input class={styles.code} readOnly value={token} onFocus={(e) => e.currentTarget.select()} />
          </Field>
          <Button icon="copy" onClick={() => copy(token, 'Token')}>Copy</Button>
        </div>
        <Field label="Container environment">
          <Textarea class={styles.code} readOnly rows={3} value={environment} onFocus={(e) => e.currentTarget.select()} />
        </Field>
        <Button icon="copy" onClick={() => copy(environment, 'Environment')}>Copy environment</Button>
      </div> : editing ? <form id={form} class={formStyles.form} noValidate onSubmit={(e) => { e.preventDefault(); void save() }}>
        {mode === 'add' && <Field label="Name" hint="Letters, digits, underscores or hyphens.">
          <Input required autoComplete="off" value={name} disabled={busy} onInput={(e) => setName(e.currentTarget.value)} />
        </Field>}
        <Field label="Neko URL" hint="Where the server reaches the room's neko (http or https, optionally with a path). Only the server uses it; it is not shown again.">
          <Input required type="url" placeholder="http://10.0.0.2:8080" value={nekoUrl} disabled={busy} onInput={(e) => setNekoUrl(e.currentTarget.value)} />
        </Field>
        {mode === 'address' && <Notice>Changing the address disconnects people in the room. They can reopen it to reconnect.</Notice>}
      </form> : mode === 'token' ? <>
        <p>Replace the token for {name}? The current token will stop working and people in the room are disconnected.</p>
        <p>The room’s container must be restarted with the new token.</p>
      </> : <>
        <p>Remove {name}? People in the room are disconnected.</p>
        <p>Chat history, settings and permissions are kept and return if a room with this name is added again.</p>
      </>}
      {error && <Notice tone="error">{error}</Notice>}
      {message && <Notice tone="success">{message}</Notice>}
    </Modal>
  )
}
