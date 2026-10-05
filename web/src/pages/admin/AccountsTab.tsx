import { useEffect, useState } from 'preact/hooks'
import { api, type AdminUser } from '../../api'
import { me } from '../../app/state'
import { Button } from '../../components/Button'
import { Modal } from '../../components/Modal'
import { AccountRow } from '../../components/admin/AccountRow'
import { AdminTable } from '../../components/admin/AdminTable'
import { ResetPasswordModal } from '../../components/admin/ResetPasswordModal'
import { Spinner } from '../../components/ui/Spinner'
import { Input } from '../../components/ui/Field'
import { Notice } from '../../components/ui/Notice'
import { Section } from '../../components/ui/Section'
import styles from './AccountsTab.module.css'

export function AccountsTab() {
  const [users, setUsers] = useState<AdminUser[]>([]),
    [error, setError] = useState(''),
    [status, setStatus] = useState('')
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true)
  const [deleting, setDeleting] = useState<string | null>(null),
    [resetting, setResetting] = useState<string | null>(null)
  const [search, setSearch] = useState('')
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
      setStatus(`${username} updated.`)
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
      setStatus(`${username} deleted.`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
      setDeleting(null)
    } finally {
      setBusy(false)
    }
  }
  const query = search.trim().toLowerCase()
  const shown = query
    ? users.filter((user) => user.username.toLowerCase().includes(query) || user.nickname.toLowerCase().includes(query))
    : users
  return (
    <Section
      title="Accounts"
      description={loading ? 'Everyone registered on this server.' : `${users.length} ${users.length === 1 ? 'account' : 'accounts'} on this server.`}
      actions={<Input class={styles.search} type="search" placeholder="Search accounts" aria-label="Search accounts"
        value={search} onInput={(e) => setSearch(e.currentTarget.value)} />}
    >
      {loading && <Spinner label="Loading accounts…" />}
      {error && <Notice tone="error">{error}</Notice>}
      {status && <Notice tone="success">{status}</Notice>}
      {!loading && <AdminTable headings={['Account', 'Verified', 'Admin', 'Enabled', 'Actions']}>
        {shown.map((user) => (
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
        {!error && shown.length === 0 && (
          <tr>
            <td colSpan={5} class={styles.empty}>{query ? `No account matches "${search.trim()}".` : 'No accounts'}</td>
          </tr>
        )}
      </AdminTable>}
      {deleting && (
        <Modal size="sm" title="Delete account" onClose={() => setDeleting(null)} footer={<>
          <Button disabled={busy} onClick={() => setDeleting(null)}>
            Cancel
          </Button>
          <Button variant="danger" disabled={busy} onClick={() => remove(deleting)}>
            Delete
          </Button>
        </>}>
          <p>Are you sure you want to delete {deleting}?</p>
          <p>Their sessions and room permissions are deleted with the account. This cannot be undone.</p>
        </Modal>
      )}
      {resetting && (
        <ResetPasswordModal
          username={resetting}
          onClose={() => setResetting(null)}
          onSaved={() => {
            setStatus(`Password of ${resetting} reset.`)
            setResetting(null)
          }}
        />
      )}
    </Section>
  )
}
