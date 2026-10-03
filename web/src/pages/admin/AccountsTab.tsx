import { useEffect, useState } from 'preact/hooks'
import { api, type AdminUser } from '../../api'
import { me } from '../../app/state'
import { Button } from '../../components/Button'
import { Modal } from '../../components/Modal'
import { AccountRow } from '../../components/admin/AccountRow'
import { AdminTable } from '../../components/admin/AdminTable'
import { ResetPasswordModal } from '../../components/admin/ResetPasswordModal'
import styles from './AccountsTab.module.css'

export function AccountsTab() {
  const [users, setUsers] = useState<AdminUser[]>([]),
    [error, setError] = useState(''),
    [status, setStatus] = useState('')
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true)
  const [deleting, setDeleting] = useState<string | null>(null),
    [resetting, setResetting] = useState<string | null>(null)
  useEffect(() => {
    let active = true
    void api
      .get<AdminUser[]>('/api/admin/users')
      .then((res) => {
        if (active) setUsers(res)
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
  }, [])
  async function update(
    username: string,
    change: Partial<Pick<AdminUser, 'verified' | 'admin' | 'disabled'>>,
  ) {
    setBusy(true)
    setError('')
    setStatus('')
    try {
      const res = await api.patch<AdminUser>(
        `/api/admin/users/${encodeURIComponent(username)}`,
        change,
      )
      setUsers((list) =>
        list.map((user) => (user.username === username ? res : user)),
      )
      setStatus('Account updated!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function remove(username: string) {
    setBusy(true)
    setError('')
    setStatus('')
    try {
      await api.del(`/api/admin/users/${encodeURIComponent(username)}`)
      setUsers((list) => list.filter((user) => user.username !== username))
      setDeleting(null)
      setStatus('Account deleted!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
      setDeleting(null)
    } finally {
      setBusy(false)
    }
  }
  return (
    <>
      {loading && <p role="status">Loading accounts...</p>}
      {error && <p role="alert">{error}</p>}
      {status && <p role="status">{status}</p>}
      <AdminTable
        headings={[
          'Avatar',
          'Username',
          'Nickname',
          'Color',
          'Verified',
          'Admin',
          'Disabled',
          'Delete',
          'Password',
        ]}
      >
        {users.map((user) => (
          <AccountRow
            key={user.username}
            user={user}
            self={user.username === me.value?.username}
            busy={busy}
            onUpdate={update}
            onDelete={(username) => {
              setError('')
              setDeleting(username)
            }}
            onReset={setResetting}
          />
        ))}
        {!loading && !error && users.length === 0 && (
          <tr>
            <td colSpan={9}>No accounts</td>
          </tr>
        )}
      </AdminTable>
      {deleting && (
        <Modal compact title="Delete account" onClose={() => setDeleting(null)}>
          <p>Are you sure you want to delete {deleting}?</p>
          <div class={styles.actions}>
            <Button accent disabled={busy} onClick={() => remove(deleting)}>
              Delete
            </Button>
            <Button disabled={busy} onClick={() => setDeleting(null)}>
              Cancel
            </Button>
          </div>
        </Modal>
      )}
      {resetting && (
        <ResetPasswordModal
          username={resetting}
          onClose={() => setResetting(null)}
          onSaved={() => {
            setResetting(null)
            setStatus('Password reset!')
          }}
        />
      )}
    </>
  )
}
