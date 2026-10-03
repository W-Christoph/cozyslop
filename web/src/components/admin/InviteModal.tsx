import { useState } from 'preact/hooks'
import { api, type InviteView } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import styles from './InviteModal.module.css'

export function InviteModal({
  room,
  onClose,
}: {
  room: string
  onClose: () => void
}) {
  const [temporary, setTemporary] = useState(false),
    [remote, setRemote] = useState(false),
    [image, setImage] = useState(false),
    [upload, setUpload] = useState(false)
  const [name, setName] = useState(''),
    [maxUses, setMaxUses] = useState('1'),
    [expiry, setExpiry] = useState('5')
  const [link, setLink] = useState(''),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false)
  async function generate() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const invite = await api.post<InviteView>('/api/admin/invites', {
        room,
        temporary,
        name,
        remote,
        image,
        upload,
        maxUses: maxUses === '' ? null : Number(maxUses),
        expiresInMinutes: expiry === '' ? null : Number(expiry),
      })
      setLink(location.origin + invite.path)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function copy() {
    setError('')
    setMessage('')
    try {
      await navigator.clipboard.writeText(link)
      setMessage('Copied!')
    } catch {
      setError('Could not copy the link. Select it and copy it manually.')
    }
  }
  return (
    <Modal title="Invite Link" onClose={onClose}>
      <form
        class={styles.form}
        onSubmit={(e) => {
          e.preventDefault()
          void generate()
        }}
      >
        <label class={styles.row}>
          <input
            type="checkbox"
            checked={temporary}
            onChange={(e) => setTemporary(e.currentTarget.checked)}
          />
          Temporary
        </label>
        <label class={styles.row}>
          <input
            type="checkbox"
            checked={remote}
            onChange={(e) => setRemote(e.currentTarget.checked)}
          />
          Allow remote rights
        </label>
        <label class={styles.row}>
          <input
            type="checkbox"
            checked={image}
            onChange={(e) => setImage(e.currentTarget.checked)}
          />
          Allow image rights
        </label>
        <label class={styles.row}>
          <input
            type="checkbox"
            checked={upload}
            onChange={(e) => setUpload(e.currentTarget.checked)}
          />
          Allow upload rights
        </label>
        <label class={styles.row}>
          Max Uses
          <select
            value={maxUses}
            onChange={(e) => setMaxUses(e.currentTarget.value)}
          >
            <option value="1">1</option>
            <option value="5">5</option>
            <option value="10">10</option>
            <option value="">Unlimited</option>
          </select>
        </label>
        <label class={styles.row}>
          Expiration
          <select
            value={expiry}
            onChange={(e) => setExpiry(e.currentTarget.value)}
          >
            <option value="5">5 minutes</option>
            <option value="60">1 hour</option>
            <option value="1440">1 day</option>
            <option value="">Unlimited</option>
          </select>
        </label>
        <label class={styles.row}>
          Invite name (optional)
          <input
            maxLength={64}
            value={name}
            onInput={(e) => setName(e.currentTarget.value)}
          />
        </label>
        <label class={styles.row}>
          Code
          <input
            class={styles.code}
            readOnly
            value={link || 'Press Generate'}
            onFocus={(e) => e.currentTarget.select()}
          />
        </label>
        <div class={styles.actions}>
          <Button type="submit" disabled={busy}>
            Generate
          </Button>
          {link && <Button onClick={copy}>Copy</Button>}
        </div>
      </form>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </Modal>
  )
}
