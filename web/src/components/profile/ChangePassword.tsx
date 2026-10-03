import { useState } from 'preact/hooks'
import { api } from '../../api'
import { Button } from '../Button'
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
      setMessage('Password changed!')
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
      <h2>Change password</h2>
      <label>
        Current password
        <input
          required
          type="password"
          autoComplete="current-password"
          maxLength={100}
          value={current}
          onInput={(e) => setCurrent(e.currentTarget.value)}
        />
      </label>
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
      <Button disabled={busy} type="submit">
        Change password
      </Button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </form>
  )
}
