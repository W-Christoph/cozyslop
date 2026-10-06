import { useEffect, useState } from 'preact/hooks'
import { useRoute } from 'preact-iso'
import { api, ApiError, type PasswordResetCheck } from '../api'
import { AuthLayout } from '../components/PageLayout'
import { InfoScreen } from '../components/InfoScreen'
import { Button } from '../components/Button'
import { Field, Input } from '../components/ui/Field'
import { Notice } from '../components/ui/Notice'
import formStyles from '../components/ui/Form.module.css'

export function ResetPasswordPage() {
  const { params: { token } } = useRoute()
  const [username, setUsername] = useState(''),
    [checking, setChecking] = useState(true),
    [invalid, setInvalid] = useState(false)
  const [password, setPassword] = useState(''),
    [confirmation, setConfirmation] = useState(''),
    [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  useEffect(() => {
    let active = true
    setChecking(true)
    setInvalid(false)
    setUsername('')
    setPassword('')
    setConfirmation('')
    setError('')
    void api.post<PasswordResetCheck>('/api/auth/password-reset/check', { token })
      .then((result) => { if (active) setUsername(result.username) })
      .catch((e) => {
        if (!active) return
        if (e instanceof ApiError && e.status === 404) setInvalid(true)
        else setError(e instanceof Error ? e.message : 'Something went wrong.')
      })
      .finally(() => { if (active) setChecking(false) })
    return () => { active = false }
  }, [token])
  async function save() {
    setError('')
    if (password !== confirmation) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      await api.post('/api/auth/password-reset/redeem', { token, password })
      // Reload to clear any revoked session from the app's cached account.
      location.replace('/login?passwordReset=1')
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setInvalid(true)
      else setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  if (checking) return <InfoScreen busy message="Checking reset link" submessage="Please wait" />
  if (invalid) return <InfoScreen icon="alert" message="Reset link not usable"
    submessage="This link is invalid or has expired. Ask a moderator for a new reset link." />
  if (!username) return <InfoScreen icon="alert" message="Could not check reset link" submessage={error} />
  return (
    <AuthLayout title={`Set a new password for ${username}`}>
      <form class={formStyles.form} onSubmit={(e) => { e.preventDefault(); void save() }}>
        <Field label="New password" hint="At least 8 characters and at most 72 bytes.">
          <Input required type="password" autoComplete="new-password" minLength={8} maxLength={72}
            value={password} onInput={(e) => setPassword(e.currentTarget.value)} />
        </Field>
        <Field label="Repeat new password">
          <Input required type="password" autoComplete="new-password" maxLength={72}
            value={confirmation} onInput={(e) => setConfirmation(e.currentTarget.value)} />
        </Field>
        {error && <Notice tone="error">{error}</Notice>}
        <Button variant="primary" size="lg" block type="submit" disabled={busy}>
          {busy ? 'Saving…' : 'Set password'}
        </Button>
      </form>
    </AuthLayout>
  )
}
