import { useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { api, type Me } from '../api'
import { logout, me, pendingInvite } from '../app/state'
import { PageLayout } from '../components/PageLayout'
import { Button } from '../components/Button'

export function LoginPage() {
  const { route } = useLocation()
  const [username, setUsername] = useState(''),
    [password, setPassword] = useState('')
  const [error, setError] = useState(''),
    [busy, setBusy] = useState(false)
  async function submit() {
    setBusy(true)
    setError('')
    try {
      const res = await api.post<{ user: Me }>('/api/auth/login', {
        username,
        password,
      })
      me.value = res.user
      const code = pendingInvite.get()
      route(code ? `/invite/${encodeURIComponent(code)}` : '/')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function signOut() {
    setBusy(true)
    setError('')
    try {
      await logout()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <PageLayout title={me.value ? 'Logged in' : 'Login'}>
      {me.value ? (
        <Button disabled={busy} onClick={signOut}>
          Logout
        </Button>
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
              name="username"
              autoComplete="username"
              required
              maxLength={12}
              value={username}
              onInput={(e) => setUsername(e.currentTarget.value)}
            />
          </label>
          <label>
            Password
            <input
              name="password"
              type="password"
              autoComplete="current-password"
              required
              maxLength={100}
              value={password}
              onInput={(e) => setPassword(e.currentTarget.value)}
            />
          </label>
          <Button type="submit" disabled={busy}>
            Login
          </Button>
        </form>
      )}
      {error && <p role="alert">{error}</p>}
    </PageLayout>
  )
}
