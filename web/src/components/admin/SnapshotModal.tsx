import { useEffect, useRef, useState } from 'preact/hooks'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Notice } from '../ui/Notice'
import { Spinner } from '../ui/Spinner'
import { fileTime, saveText } from './diagFormat'
import styles from './SnapshotModal.module.css'

// The server's state now, as text to copy into a message or save: the
// machine's memory, the server's, each room's, and the last hours.
export function SnapshotModal({ load, onClose }: { load: () => Promise<string>; onClose: () => void }) {
  const [text, setText] = useState(''),
    [error, setError] = useState(''),
    [copied, setCopied] = useState<'' | 'yes' | 'select'>('')
  const box = useRef<HTMLPreElement>(null)
  useEffect(() => {
    let active = true
    void load().then((t) => { if (active) setText(t) })
      .catch((e) => { if (active) setError(e instanceof Error ? e.message : 'Something went wrong.') })
    return () => { active = false }
  }, [])

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied('yes')
    } catch {
      // Without HTTPS the browser has no clipboard for pages: select it.
      const range = document.createRange()
      range.selectNodeContents(box.current!)
      getSelection()?.removeAllRanges()
      getSelection()?.addRange(range)
      setCopied('select')
    }
  }

  return (
    <Modal title="Snapshot" size="xl" onClose={onClose} footer={<>
      <Button icon="download" disabled={!text} onClick={() => saveText(text, `cozycast-snapshot-${fileTime()}.txt`)}>Download</Button>
      <Button variant="primary" icon={copied === 'yes' ? 'check' : 'copy'} disabled={!text} onClick={copy}>
        {copied === 'yes' ? 'Copied' : 'Copy'}
      </Button>
    </>}>
      <p class={styles.about}>Where the memory is now (the machine, the server, each room and its programs) and the last hours,
        as text to paste into a message. It names no one.</p>
      {error && <Notice tone="error">{error}</Notice>}
      {copied === 'select' && <Notice tone="info">The browser does not allow copying here: the text is selected, press Ctrl+C.</Notice>}
      {!text && !error && <Spinner label="Taking a snapshot…" />}
      {text && <pre ref={box} class={styles.text} tabIndex={0}>{text}</pre>}
    </Modal>
  )
}
