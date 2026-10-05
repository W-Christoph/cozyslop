import { useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { me, pendingInvite, register, serverSettings } from '../app/state'
import { AuthLayout } from '../components/PageLayout'
import { Button, ButtonLink } from '../components/Button'
import { Field, Input } from '../components/ui/Field'
import formStyles from '../components/ui/Form.module.css'
import { Notice } from '../components/ui/Notice'

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
      await register(username, password, inviteCode)
      pendingInvite.clear()
      route('/')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  if (me.value)
    return (
      <AuthLayout title="You're logged in" subtitle={`Signed in as ${me.value.username}.`}>
        <ButtonLink variant="primary" size="lg" block href="/">
          Go to rooms
        </ButtonLink>
      </AuthLayout>
    )
  const footer = <>Already have an account? <a href="/login">Log in</a></>
  if (inviteOnly)
    return (
      <AuthLayout title="Invite required" subtitle="Registration requires an invite." footer={footer}>
        <Notice>Ask an admin of this server for an invite link, then open it to create your account.</Notice>
      </AuthLayout>
    )
  return (
    <AuthLayout title="Create an account" subtitle="Join movie night on CozyCast" footer={footer}>
      <form
        class={formStyles.form}
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
      >
        <Field label="Username" hint="2 to 12 characters.">
          <Input
            autoComplete="username"
            autoFocus
            required
            minLength={2}
            maxLength={12}
            value={username}
            onInput={(e) => setUsername(e.currentTarget.value)}
          />
        </Field>
        <Field label="Password" hint="At least 8 characters.">
          <Input
            type="password"
            autoComplete="new-password"
            required
            minLength={8}
            maxLength={72}
            value={password}
            onInput={(e) => setPassword(e.currentTarget.value)}
          />
        </Field>
        <Field label="Confirm password">
          <Input
            type="password"
            autoComplete="new-password"
            required
            maxLength={72}
            value={confirmation}
            onInput={(e) => setConfirmation(e.currentTarget.value)}
          />
        </Field>
        <Field label="Invite code" hint={pending ? 'From your invite link.' : 'Optional.'}>
          <Input
            readOnly={!!pending}
            value={inviteCode}
            onInput={(e) => setInviteCode(e.currentTarget.value)}
          />
        </Field>
        {error && <Notice tone="error">{error}</Notice>}
        <Button variant="primary" size="lg" block type="submit" disabled={busy}>
          {busy ? 'Creating account…' : 'Sign up'}
        </Button>
      </form>
    </AuthLayout>
  )
}
