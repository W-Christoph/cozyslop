import { useId, useState } from 'preact/hooks'
import { api } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Field, Input } from '../ui/Field'
import { Notice } from '../ui/Notice'
import styles from './ResetPasswordModal.module.css'

export function ResetPasswordModal({
  username,
  onClose,
  onSaved,
}: {
  username: string
  onClose: () => void
  onSaved: () => void
}) {
  const [password, setPassword] = useState(''),
    [confirmation, setConfirmation] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const form = useId()
  async function save() {
    setError('')
    if (password !== confirmation) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post(
        `/api/admin/users/${encodeURIComponent(username)}/password`,
        { password },
      )
      onSaved()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal compact title={`Reset password: ${username}`} onClose={onClose} footer={<>
      <Button onClick={onClose}>Cancel</Button>
      <Button accent type="submit" form={form} disabled={busy}>
        Reset password
      </Button>
    </>}>
      <form
        id={form}
        class={styles.form}
        onSubmit={(e) => {
          e.preventDefault()
          void save()
        }}
      >
        <Notice>{username} is logged out everywhere and removed from rooms.</Notice>
        <Field label="New password" hint="At least 8 characters.">
          <Input
            required
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={72}
            value={password}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </Field>
        <Field label="Confirm password">
          <Input
            required
            type="password"
            autoComplete="new-password"
            maxLength={72}
            value={confirmation}
            onInput={(e) => setConfirmation(e.currentTarget.value)}
          />
        </Field>
        {error && <Notice tone="error">{error}</Notice>}
      </form>
    </Modal>
  )
}
