import { useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { api, type Me } from '../api'
import { me, pendingInvite, serverSettings } from '../app/state'
import { PageLayout } from '../components/PageLayout'
import { Button } from '../components/Button'

export function RegisterPage() {
  const { route } = useLocation()
  const [pending] = useState(() => pendingInvite.get())
  const [inviteCode, setInviteCode] = useState(pending ?? '')
  const [username, setUsername] = useState(''),
    [password, setPassword] = useState(''),
    [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  const inviteOnly = serverSettings.value.registration === 'invite' && !pending
  async function submit() {
    setError('')
    if (password !== confirmation) {
      setError('Passwords do not match')
      return
    }
    setBusy(true)
    try {
      const res = await api.post<{ user: Me }>('/api/auth/register', {
        username,
        password,
        ...(inviteCode ? { inviteCode } : {}),
      })
      me.value = res.user
      pendingInvite.clear()
      route('/')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <PageLayout title={me.value ? 'Logged in' : 'Register'}>
      {me.value ? (
        <a href="/">Home</a>
      ) : inviteOnly ? (
        <p>Registration requires an invite.</p>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <label>
            Username
            <input
              autoComplete="username"
              required
              minLength={2}
              maxLength={12}
              value={username}
              onInput={(e) => setUsername(e.currentTarget.value)}
            />
          </label>
          <label>
            Password
            <input
              type="password"
              autoComplete="new-password"
              required
              minLength={8}
              maxLength={72}
              value={password}
              onInput={(e) => setPassword(e.currentTarget.value)}
            />
          </label>
          <label>
            Confirm Password
            <input
              type="password"
              autoComplete="new-password"
              required
              maxLength={72}
              value={confirmation}
              onInput={(e) => setConfirmation(e.currentTarget.value)}
            />
          </label>
          <label>
            Invite Code
            <input
              readOnly={!!pending}
              value={inviteCode}
              onInput={(e) => setInviteCode(e.currentTarget.value)}
            />
          </label>
          <Button type="submit" disabled={busy}>
            Register
          </Button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </PageLayout>
  )
}
