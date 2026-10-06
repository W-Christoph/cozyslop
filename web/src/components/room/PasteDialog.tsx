import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Checkbox } from '../ui/Field'
import styles from './PasteDialog.module.css'

const PREVIEW_LENGTH = 2000

export function PasteDialog({ text, onAccept, onCancel }: {
  text: string
  onAccept: (dontAsk: boolean) => void
  onCancel: () => void
}) {
  const [dontAsk, setDontAsk] = useState(false)
  const form = useRef<HTMLFormElement>(null)
  const characters = Array.from(text)
  const remaining = Math.max(0, characters.length - PREVIEW_LENGTH)
  useLayoutEffect(() => { form.current?.querySelector<HTMLButtonElement>('button[type="submit"]')?.focus() }, [])
  return <Modal title="Paste into the desktop?" size="sm" onClose={onCancel}>
    <form ref={form} onSubmit={(e) => { e.preventDefault(); onAccept(dontAsk) }}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && !(e.target instanceof HTMLButtonElement)) {
          e.preventDefault()
          onAccept(dontAsk)
        }
      }}>
      <p class={styles.notice}>Everyone in the room can see the desktop.</p>
      <pre class={styles.preview}>{characters.slice(0, PREVIEW_LENGTH).join('')}</pre>
      {remaining > 0 && <p class={styles.notice}>… {remaining} more characters</p>}
      <label class={styles.checkbox}>
        <Checkbox checked={dontAsk} onChange={(e) => setDontAsk(e.currentTarget.checked)} />
        Don't ask again
      </label>
      <div class={styles.actions}>
        <Button onClick={onCancel}>Cancel</Button>
        <Button type="submit" variant="primary">Paste</Button>
      </div>
    </form>
  </Modal>
}
