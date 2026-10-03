import { useEffect, useState } from 'preact/hooks'
import { api, type InviteView } from '../../api'
import { Button } from '../Button'
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
      setMessage('Invite deleted!')
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
      setMessage('Copied!')
    } catch {
      setError('Could not copy the link. Select it and copy it manually.')
    }
  }
  return (
    <>
      {loading && <p role="status">Loading invites...</p>}
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
      <AdminTable
        headings={[
          'Room',
          'Type',
          'Expired',
          'Uses',
          'Remote Permission',
          'Image Permission',
          'Upload Permission',
          'Link',
          'Name',
          'Action',
        ]}
      >
        {invites.map((invite) => (
          <tr key={invite.code}>
            <td>{invite.room}</td>
            <td>{invite.temporary ? 'Access' : 'Invite'}</td>
            <td>{invite.valid ? 'Active' : 'Expired'}</td>
            <td>
              {invite.uses} / {invite.maxUses ?? '∞'}
            </td>
            <td>{invite.remote ? 'Remote Allowed' : 'No Remote'}</td>
            <td>{invite.image ? 'Can Post Images' : 'No Images'}</td>
            <td>{invite.upload ? 'Upload Allowed' : 'No Upload'}</td>
            <td>
              <a class={styles.link} href={invite.path}>
                {location.origin + invite.path}
              </a>{' '}
              <Button onClick={() => copy(invite.path)}>Copy</Button>
            </td>
            <td>{invite.name}</td>
            <td>
              <Button
                accent
                disabled={busy}
                onClick={() => remove(invite.code)}
              >
                Delete
              </Button>
            </td>
          </tr>
        ))}
        {!loading && !error && invites.length === 0 && (
          <tr>
            <td colSpan={10}>No invites</td>
          </tr>
        )}
      </AdminTable>
    </>
  )
}
