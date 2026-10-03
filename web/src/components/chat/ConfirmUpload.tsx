import { useEffect, useRef, useState } from 'preact/hooks'
import { api } from '../../api'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { useRoomStore } from '../room/RoomContext'
import { useObjectUrl } from './useObjectUrl'
import styles from './ConfirmUpload.module.css'

export function ConfirmUpload({ file, screenshot, onClose, onUploaded, onError }: {
  file: File
  screenshot: boolean
  onClose: () => void
  onUploaded: () => void
  onError: (message: string) => void
}) {
  const store = useRoomStore()
  const source = useObjectUrl(file)
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const inFlight = useRef(false)
  const mounted = useRef(true)
  useEffect(() => () => { mounted.current = false }, [])
  async function upload() {
    if (inFlight.current || !store.rights.peek().image) return
    inFlight.current = true
    setSending(true)
    setError('')
    const form = new FormData()
    form.append('file', file)
    try {
      await api.post(`/api/rooms/${encodeURIComponent(store.room)}/media`, form)
      if (mounted.current) onUploaded()
    } catch (e) {
      const message = e instanceof Error ? e.message : 'Could not upload this file.'
      onError(message)
      if (mounted.current) setError(message)
    } finally {
      inFlight.current = false
      if (mounted.current) setSending(false)
    }
  }
  return <Modal title="Upload this file?" compact={!screenshot} onClose={onClose}>
    {source && (file.type.startsWith('video/')
      ? <video class={styles.preview} src={source} autoplay loop muted playsInline />
      : <img class={styles.preview} src={source} alt="Upload preview" />)}
    <div class={styles.actions}>
      <Button accent disabled={sending || !store.rights.value.image || store.server.value !== 'connected'} onClick={() => { void upload() }}>{sending ? 'Uploading…' : 'Upload'}</Button>
      <Button onClick={onClose}>Cancel</Button>
    </div>
    {error && <p class={styles.error} role="alert">{error}</p>}
  </Modal>
}
