import { useEffect, useState } from 'preact/hooks'
import { api, type InviteView } from '../../api'
import { Button } from '../Button'
import { Badge } from '../ui/Badge'
import { EmptyState } from '../ui/EmptyState'
import { Spinner } from '../ui/Spinner'
import { Notice } from '../ui/Notice'
import { AdminTable } from './AdminTable'
import styles from './InviteList.module.css'

export function InviteList({ room }: { room?: string }) {
  const [invites, setInvites] = useState<InviteView[]>([]),
    [error, setError] = useState(''),
    [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true)
  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    setMessage('')
    setInvites([])
    void api
      .get<InviteView[]>(
        `/api/admin/invites${room ? `?room=${encodeURIComponent(room)}` : ''}`,
      )
      .then((res) => {
        if (active) setInvites(res)
      })
      .catch((e) => {
        if (active)
          setError(e instanceof Error ? e.message : 'Something went wrong.')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [room])
  async function remove(code: string) {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      await api.del(`/api/admin/invites/${encodeURIComponent(code)}`)
      setInvites((list) => list.filter((invite) => invite.code !== code))
      setMessage('Invite deleted.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function copy(path: string) {
    setError('')
    setMessage('')
    try {
      await navigator.clipboard.writeText(location.origin + path)
      setMessage('Link copied.')
    } catch {
      setError('Could not copy the link. Select it and copy it manually.')
    }
  }
  const expires = (invite: InviteView) =>
    invite.expiresAt === null ? 'Never expires' : `${invite.valid ? 'Expires' : 'Expired'} ${new Date(invite.expiresAt * 1000).toLocaleString()}`
  return (
    <>
      {loading && <Spinner label="Loading invites…" />}
      {error && <Notice tone="error">{error}</Notice>}
      {message && <Notice tone="success">{message}</Notice>}
      {!loading && !error && invites.length === 0 && (
        <EmptyState icon="ticket" title="No invites">
          {room ? 'No invite or access link has been created for this room.' : 'No invite or access link exists.'}
        </EmptyState>
      )}
      {invites.length > 0 && (
        <AdminTable headings={[...(room ? [] : ['Room']), 'Link', 'Status', 'Uses', 'Grants', '']}>
          {invites.map((invite) => (
            <tr key={invite.code} class={invite.valid ? undefined : styles.expired}>
              {!room && <td class={styles.room}>{invite.room}</td>}
              <td class={styles.linkCell}>
                <div class={styles.name}>
                  {invite.name || <span class={styles.unnamed}>Unnamed</span>}
                  <Badge icon={invite.temporary ? 'eye' : 'ticket'}>{invite.temporary ? 'Temporary' : 'Invite'}</Badge>
                </div>
                <a class={styles.link} href={invite.path}>
                  {location.origin + invite.path}
                </a>
              </td>
              <td>
                <Badge tone={invite.valid ? 'success' : 'warning'} title={expires(invite)}>{invite.valid ? 'Active' : 'Expired'}</Badge>
                <div class={styles.expires}>{expires(invite)}</div>
              </td>
              <td class={styles.uses} data-label="Uses">
                {invite.uses} / {invite.maxUses ?? '∞'}
              </td>
              <td>
                <div class={styles.grants}>
                  {invite.remote && <Badge icon="mouse">Remote</Badge>}
                  {invite.image && <Badge icon="image">Images</Badge>}
                  {invite.upload && <Badge icon="upload">Upload</Badge>}
                  {!invite.remote && !invite.image && !invite.upload && <span class={styles.none}>Room defaults</span>}
                </div>
              </td>
              <td>
                <div class={styles.actions}>
                  <Button size="sm" variant="ghost" icon="copy" onClick={() => copy(invite.path)}>Copy link</Button>
                  <Button
                    size="sm"
                    variant="danger-ghost"
                    icon="trash"

                    aria-label={`Delete the ${invite.temporary ? 'temporary link' : 'invite'} ${invite.name || invite.code}`}
                    title="Delete"
                    disabled={busy}
                    onClick={() => remove(invite.code)}
                  />
                </div>
              </td>
            </tr>
          ))}
        </AdminTable>
      )}
    </>
  )
}
