import { useState } from 'preact/hooks'
import { api } from '../../api'
import { Button } from '../Button'
import { Field, Input } from '../ui/Field'
import { Notice } from '../ui/Notice'
import styles from './ChangePassword.module.css'

export function ChangePassword() {
  const [current, setCurrent] = useState(''),
    [password, setPassword] = useState(''),
    [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false)
  async function save() {
    setError('')
    setMessage('')
    if (password !== confirmation) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post('/api/me/password', { current, new: password })
      setCurrent('')
      setPassword('')
      setConfirmation('')
      setMessage('Password changed. Other devices have been logged out.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form
      class={styles.form}
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <Field label="Current password">
        <Input
          required
          type="password"
          autoComplete="current-password"
          maxLength={72}
          value={current}
          onInput={(e) => setCurrent(e.currentTarget.value)}
        />
      </Field>
      <div class={styles.pair}>
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
        <Field label="Confirm new password">
          <Input
            required
            type="password"
            autoComplete="new-password"
            maxLength={72}
            value={confirmation}
            onInput={(e) => setConfirmation(e.currentTarget.value)}
          />
        </Field>
      </div>
      {error && <Notice tone="error">{error}</Notice>}
      {message && <Notice tone="success">{message}</Notice>}
      <div class={styles.actions}>
        <Button disabled={busy} type="submit">
          Change password
        </Button>
      </div>
    </form>
  )
}
