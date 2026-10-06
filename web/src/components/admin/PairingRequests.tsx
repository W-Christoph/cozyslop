import { useEffect, useState } from 'preact/hooks'
import { adminPairing, type AdminRoom, type PairingRequest } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Field, Input, Select } from '../ui/Field'
import { Notice } from '../ui/Notice'
import { RadioCards } from '../ui/RadioCards'
import { Section } from '../ui/Section'
import { roomNameError } from './roomValidation'
import formStyles from '../ui/Form.module.css'
import styles from './PairingRequests.module.css'

// Requests are time-limited and someone is waiting at the other end.
const POLL_MS = 3_000

function ago(ms: number) {
  const minutes = Math.floor((Date.now() - ms) / 60_000)
  return minutes < 1 ? 'just now' : `${minutes} min ago`
}

// Computers asking to run a room (docs/home-hosting.md, "Pairing"). Shown
// only while there are any.
export function PairingRequests({ rooms, onAccepted }: {
  rooms: readonly AdminRoom[] // for giving a paired room a new computer
  onAccepted: (room: AdminRoom) => void
}) {
  const [requests, setRequests] = useState<PairingRequest[]>([]),
    [error, setError] = useState(''),
    [answer, setAnswer] = useState<{ mode: 'accept' | 'reject'; request: PairingRequest } | null>(null)
  useEffect(() => {
    let active = true
    const load = () => void adminPairing.list().then((list) => {
      if (active) { setRequests(list); setError('') }
    }).catch((e) => {
      if (active) setError(e instanceof Error ? e.message : 'Something went wrong.')
    })
    load()
    const timer = window.setInterval(load, POLL_MS)
    return () => { active = false; window.clearInterval(timer) }
  }, [])
  if (requests.length === 0 && !error) return null
  const answered = (id: string) => setRequests((list) => list.filter((r) => r.id !== id))
  return (
    <Section title="Requests" description="Computers asking to run a room. Accept only if the code matches the one its owner sees.">
      {error && <Notice tone="error">{error}</Notice>}
      <ul class={styles.list}>
        {requests.map((request) => <li key={request.id} class={styles.request}>
          <span class={styles.code} aria-label={`Code ${request.code}`}>{request.code}</span>
          <span class={styles.details}>
            <span class={styles.name}>{request.name}</span>
            <span class={styles.meta}>{request.ip} · {ago(request.createdAt)}</span>
          </span>
          <span class={styles.actions}>
            <Button size="sm" variant="primary" onClick={() => setAnswer({ mode: 'accept', request })}>Accept</Button>
            <Button size="sm" variant="danger-ghost" onClick={() => setAnswer({ mode: 'reject', request })}>Reject</Button>
          </span>
        </li>)}
      </ul>
      {answer && <PairingAnswer key={answer.request.id} {...answer} rooms={rooms} onClose={() => setAnswer(null)}
        onDone={(room) => {
          answered(answer.request.id)
          setAnswer(null)
          if (room) onAccepted(room)
        }} />}
    </Section>
  )
}

export function PairingAnswer({ mode, request, rooms, onClose, onDone }: {
  mode: 'accept' | 'reject'
  request: PairingRequest
  rooms: readonly AdminRoom[]
  onClose: () => void
  onDone: (room: AdminRoom | null) => void
}) {
  const paired = rooms.filter((r) => r.source === 'paired')
  const [target, setTarget] = useState<'new' | 'replace'>('new'),
    [name, setName] = useState(request.name),
    [replace, setReplace] = useState(paired[0]?.name ?? ''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const close = () => { if (!busy) onClose() }
  async function save() {
    if (busy) return
    setError('')
    if (mode === 'accept' && target === 'new') {
      const invalid = roomNameError(name)
      if (invalid) { setError(invalid); return }
    }
    setBusy(true)
    try {
      if (mode === 'reject') {
        await adminPairing.reject(request.id)
        onDone(null)
      } else {
        onDone(await adminPairing.accept(request.id, target === 'new' ? { name } : { replace }))
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
      setBusy(false)
    }
  }
  return (
    <Modal title={mode === 'accept' ? 'Accept computer' : 'Reject computer'} onClose={close} footer={<>
      <Button disabled={busy} onClick={close}>Cancel</Button>
      <Button variant={mode === 'accept' ? 'primary' : 'danger'} disabled={busy} onClick={save}>{mode === 'accept' ? 'Accept' : 'Reject'}</Button>
    </>}>
      <div class={formStyles.form}>
        <p class={styles.bigCode}>{request.code}</p>
        <p>Its owner must see exactly this code. If not, someone else may be asking: {mode === 'accept' ? 'cancel and reject it.' : 'reject it.'}</p>
        {mode === 'reject' ? <p>The computer will not ask again unless its owner deletes its saved state.</p> : <>
          {paired.length > 0 && <RadioCards name="pair-target" label="Room" value={target} disabled={busy} onChange={setTarget} options={[
            { value: 'new', label: 'A new room' },
            { value: 'replace', label: 'New computer for a room', description: 'Keeps the room’s chat, settings and permissions.' },
          ]} />}
          {target === 'new' ? <Field label="Room name" hint="Letters, digits, underscores or hyphens.">
            <Input required autoComplete="off" value={name} disabled={busy} onInput={(e) => setName(e.currentTarget.value)} />
          </Field> : <Field label="Room" hint="Its current computer is cut off.">
            <Select value={replace} disabled={busy} onChange={(e) => setReplace(e.currentTarget.value)}>
              {paired.map((r) => <option key={r.name} value={r.name}>{r.name}</option>)}
            </Select>
          </Field>}
        </>}
        {error && <Notice tone="error">{error}</Notice>}
      </div>
    </Modal>
  )
}
