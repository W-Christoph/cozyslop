import { useState } from 'preact/hooks'
import { api, type PasswordResetLink } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Field, Input } from '../ui/Field'
import { Notice } from '../ui/Notice'
import formStyles from '../ui/Form.module.css'
import styles from './ResetLinkModal.module.css'

export function ResetLinkModal({ username, onClose }: {
  username: string
  onClose: () => void
}) {
  const [link, setLink] = useState(''),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false)
  async function generate() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const reset = await api.post<PasswordResetLink>(
        `/api/admin/users/${encodeURIComponent(username)}/password-reset`,
      )
      setLink(location.origin + reset.path)
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
      setMessage('Link copied.')
    } catch {
      setError('Could not copy the link. Select it and copy it manually.')
    }
  }
  return (
    <Modal title={`Reset link: ${username}`} onClose={onClose} footer={<>
      <Button onClick={onClose}>{link ? 'Done' : 'Cancel'}</Button>
      <Button variant="primary" disabled={busy} onClick={generate}>
        {link ? 'Generate another' : 'Generate link'}
      </Button>
    </>}>
      <div class={formStyles.form}>
        <Notice>The link is valid for 24 hours, works once, and replaces earlier links. Send it privately to {username}. Using it logs them out everywhere.</Notice>
        {link && <Field label="Link">
          <div class={styles.link}>
            <Input class={styles.code} readOnly value={link} onFocus={(e) => e.currentTarget.select()} />
            <Button icon="copy" onClick={copy}>Copy</Button>
          </div>
        </Field>}
        {error && <Notice tone="error">{error}</Notice>}
        {message && <Notice tone="success">{message}</Notice>}
      </div>
    </Modal>
  )
}
