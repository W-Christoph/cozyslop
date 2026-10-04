import { useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { login, logout, me, pendingInvite, serverSettings } from '../app/state'
import { AuthLayout } from '../components/PageLayout'
import { Button, ButtonLink } from '../components/Button'
import { Field, Input } from '../components/ui/Field'
import { Notice } from '../components/ui/Notice'

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
      await login(username, password)
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
  if (me.value)
    return (
      <AuthLayout title="You're logged in" subtitle={`Signed in as ${me.value.username}.`}>
        <ButtonLink accent size="lg" block href="/">
          Go to rooms
        </ButtonLink>
        <Button size="lg" block disabled={busy} onClick={signOut}>
          Log out
        </Button>
        {error && <Notice tone="error">{error}</Notice>}
      </AuthLayout>
    )
  const open = serverSettings.value.registration === 'open' || !!pendingInvite.get()
  return (
    <AuthLayout
      title="Welcome back"
      subtitle="Log in to CozyCast"
      footer={open ? <>No account yet? <a href="/register">Sign up</a></> : undefined}
    >
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <Field label="Username">
          <Input
            name="username"
            autoComplete="username"
            autoFocus
            required
            maxLength={12}
            value={username}
            onInput={(e) => setUsername(e.currentTarget.value)}
          />
        </Field>
        <Field label="Password">
          <Input
            name="password"
            type="password"
            autoComplete="current-password"
            required
            maxLength={100}
            value={password}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </Field>
        {error && <Notice tone="error">{error}</Notice>}
        <Button accent size="lg" block type="submit" disabled={busy}>
          {busy ? 'Logging in…' : 'Log in'}
        </Button>
      </form>
    </AuthLayout>
  )
}
