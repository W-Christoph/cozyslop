import { useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { logout, me, serverSettings } from '../app/state'
import styles from './Header.module.css'

export function Header() {
  const { path, route } = useLocation()
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  async function signOut() {
    setBusy(true)
    setError('')
    try {
      await logout()
      route('/login')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  const active = (href: string) => (path === href ? styles.active : undefined)
  return (
    <header class={styles.header}>
      <h1>CozyCast</h1>
      {me.value && (
        <a
          href="/profile"
          class={`${styles.avatar} ${active('/profile') ?? ''}`}
          aria-label="Profile"
        >
          <img
            src={me.value.avatarUrl || '/png/default_avatar.png'}
            alt="Your avatar"
          />
        </a>
      )}
      <nav aria-label="Main navigation">
        <a class={active('/')} href="/">
          Rooms
        </a>
        <a class={active('/settings')} href="/settings">
          Settings
        </a>
        {me.value ? (
          <button disabled={busy} onClick={signOut}>
            Logout
          </button>
        ) : (
          <a class={active('/login')} href="/login">
            Login
          </a>
        )}
        {me.value?.admin && (
          <a
            class={path.startsWith('/admin') ? styles.active : undefined}
            href="/admin/settings"
          >
            Admin
          </a>
        )}
        {!me.value && serverSettings.value.registration === 'open' && (
          <a class={active('/register')} href="/register">
            Register
          </a>
        )}
      </nav>
      {error && (
        <div class={styles.error} role="alert">
          {error}
        </div>
      )}
    </header>
  )
}
