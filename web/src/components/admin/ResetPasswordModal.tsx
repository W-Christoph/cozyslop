import { useState } from 'preact/hooks'
import { api } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'

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
    <Modal compact title={`Reset password: ${username}`} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void save()
        }}
      >
        <label>
          New password
          <input
            required
            type="password"
            autoComplete="new-password"
            minLength={8}
            maxLength={100}
            value={password}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </label>
        <label>
          Confirm Password
          <input
            required
            type="password"
            autoComplete="new-password"
            maxLength={100}
            value={confirmation}
            onInput={(e) => setConfirmation(e.currentTarget.value)}
          />
        </label>
        <Button type="submit" disabled={busy}>
          Reset password
        </Button>
      </form>
      {error && <p role="alert">{error}</p>}
    </Modal>
  )
}
