import { useEffect, useState } from 'preact/hooks'
import { api, type ServerSettings } from '../../api'
import { refreshServerSettings, serverSettings } from '../../app/state'
import { Button } from '../../components/Button'
import styles from './ServerSettingsTab.module.css'

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
      setStatus('updated!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form
      class={styles.settings}
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <label class={styles.message}>
        Message
        <textarea
          rows={3}
          cols={60}
          maxLength={4096}
          value={message}
          onInput={(e) => setMessage(e.currentTarget.value)}
        />
      </label>
      <label>
        <input
          type="checkbox"
          checked={inviteOnly}
          onChange={(e) => setInviteOnly(e.currentTarget.checked)}
        />
        Invite required to register
      </label>
      <Button accent type="submit" disabled={busy}>
        Update Settings
      </Button>
      {error && <p role="alert">{error}</p>}
      {status && <p role="status">{status}</p>}
    </form>
  )
}
