import { useEffect, useState } from 'preact/hooks'
import { api, type ServerSettings } from '../../api'
import { refreshServerSettings, serverSettings } from '../../app/state'
import { Button } from '../../components/Button'
import { Field, Textarea } from '../../components/ui/Field'
import { FormActions } from '../../components/ui/FormActions'
import { Section, ToggleRow } from '../../components/ui/Section'

export function ServerSettingsTab() {
  const settings = serverSettings.value
  const [message, setMessage] = useState(settings.message),
    [inviteOnly, setInviteOnly] = useState(settings.registration === 'invite')
  const [error, setError] = useState(''),
    [status, setStatus] = useState(''),
    [busy, setBusy] = useState(false)
  useEffect(() => {
    setMessage(settings.message)
    setInviteOnly(settings.registration === 'invite')
  }, [settings])
  async function save() {
    setBusy(true)
    setError('')
    setStatus('')
    try {
      serverSettings.value = await api.put<ServerSettings>(
        '/api/admin/settings',
        { message, registration: inviteOnly ? 'invite' : 'open' },
      )
      await refreshServerSettings()
      setStatus('Settings saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Section title="Server settings">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void save()
        }}
      >
        <Section title="Announcement" description="Shown to everyone above the list of rooms.">
          <Field label="Message" hint="Leave empty to show nothing.">
            <Textarea
              rows={4}
              maxLength={4096}
              value={message}
              onInput={(e) => { setMessage(e.currentTarget.value); setStatus('') }}
            />
          </Field>
        </Section>
        <Section title="Registration">
          <ToggleRow title="Invite required to register" description="Only people with an invite link can create an account."
            checked={inviteOnly} onChange={(value) => { setInviteOnly(value); setStatus('') }} />
        </Section>
        <FormActions error={error} message={status}>
          <Button variant="primary" type="submit" disabled={busy}>
            {busy ? 'Saving…' : 'Save changes'}
          </Button>
        </FormActions>
      </form>
    </Section>
  )
}
