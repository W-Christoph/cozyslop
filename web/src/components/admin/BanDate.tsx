import { useRef, useState } from 'preact/hooks'
import { Button } from '../Button'
import { Field, Input } from '../ui/Field'
import styles from './BanDate.module.css'

export function BanDate({ banned, until, busy, name, onUntil }: {
  banned: boolean
  until: string
  busy: boolean
  name: string
  onUntil: (value: string) => void
}) {
  const [open, setOpen] = useState(false)
  const trigger = useRef<HTMLButtonElement>(null)
  function close() {
    setOpen(false)
    trigger.current?.focus()
  }
  return <div class={styles.date}>
    <button type="button" class={styles.chip} ref={trigger} disabled={busy}
      aria-label={`Ban date for ${name}`} aria-expanded={open}
      onClick={() => setOpen((value) => !value)}>
      {!banned ? 'No ban' : until ? until.replace('T', ' ') : 'Forever'}
    </button>
    {open && <div class={styles.editor} onKeyDown={(e) => {
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); close() }
    }}>
      <Field label="Banned until" hint="Empty means forever.">
        <Input compact type="datetime-local" aria-label={`Banned until for ${name} (empty means forever)`}
          value={until} disabled={busy} autoFocus onInput={(e) => onUntil(e.currentTarget.value)} />
      </Field>
      <Button size="sm" variant="ghost" disabled={busy} onClick={close}>Done</Button>
    </div>}
  </div>
}
