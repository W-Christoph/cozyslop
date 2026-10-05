import { useEffect, useRef, useState } from 'preact/hooks'
import { useLocation } from 'preact-iso'
import { logout, me, serverSettings, settingsOpen } from '../app/state'
import { ButtonLink } from './Button'
import { Avatar } from './ui/Avatar'
import { Icon } from './ui/Icon'
import { Notice } from './ui/Notice'
import styles from './Header.module.css'

export function Header() {
  const { path } = useLocation()
  const user = me.value
  const link = (href: string, label: string, current = path === href) => (
    <a class={`${styles.link} ${current ? styles.current : ''}`} href={href} aria-current={current ? 'page' : undefined}>{label}</a>
  )
  return (
    <header data-ui class={styles.header}>
      <a class={styles.brand} href="/">
        <img src="/png/favicon.png" alt="" width={32} height={32} />
        <span>CozyCast</span>
      </a>
      <nav class={styles.nav} aria-label="Main navigation">
        {link('/', 'Rooms')}
        {user?.admin && link('/admin/accounts', 'Admin', path.startsWith('/admin'))}
      </nav>
      <div class={styles.actions}>
        <button type="button" class={styles.iconButton} aria-label="Settings" title="Settings"
          onClick={() => { settingsOpen.value = 'appearance' }}>
          <Icon name="sliders" size={20} />
        </button>
        {user ? <UserMenu /> : <>
          <ButtonLink variant="ghost" href="/login">Log in</ButtonLink>
          {serverSettings.value.registration === 'open' && <ButtonLink variant="primary" href="/register">Sign up</ButtonLink>}
        </>}
      </div>
    </header>
  )
}

function UserMenu() {
  const { route } = useLocation()
  const user = me.value
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const menu = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const trigger = menu.current?.querySelector<HTMLElement>('button')
    const close = () => { setOpen(false); trigger?.focus() }
    const outside = (e: MouseEvent) => { if (!menu.current?.contains(e.target as Node)) close() }
    const items = () => [...(menu.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not(:disabled)') ?? [])]
    items()[0]?.focus()
    // Arrow keys and Tab stay in the menu; Escape returns to its button.
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        close()
      } else if (e.key === 'Tab') {
        e.preventDefault()
        const all = items()
        const at = all.indexOf(document.activeElement as HTMLElement)
        all[(at + (e.shiftKey ? -1 : 1) + all.length) % all.length]?.focus()
      }
      else if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault()
        const all = items()
        const at = all.indexOf(document.activeElement as HTMLElement)
        all[(at + (e.key === 'ArrowDown' ? 1 : -1) + all.length) % all.length]?.focus()
      }
    }
    document.addEventListener('mousedown', outside)
    document.addEventListener('keydown', key)
    return () => {
      document.removeEventListener('mousedown', outside)
      document.removeEventListener('keydown', key)
    }
  }, [open])
  if (!user) return null
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
  const openSettings = (section: 'account' | 'appearance') => {
    menu.current?.querySelector<HTMLElement>('button')?.focus()
    setOpen(false)
    settingsOpen.value = section
  }
  return (
    <div class={styles.user} ref={menu}>
      <button type="button" class={styles.userButton} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
        <Avatar src={user.avatarUrl} size={30} />
        <span class={styles.userName}>{user.nickname}</span>
        <Icon name="chevronDown" size={16} />
      </button>
      {open && (
        <div class={styles.menu} role="menu">
          <div class={styles.menuHead}>
            <Avatar src={user.avatarUrl} size={40} />
            <div>
              <div class={styles.menuName}>{user.nickname}</div>
              <div class={styles.menuUser}>{user.username}</div>
            </div>
          </div>
          <button type="button" role="menuitem" class={styles.menuItem} onClick={() => openSettings('account')}>
            <Icon name="user" />My account
          </button>
          <button type="button" role="menuitem" class={styles.menuItem} onClick={() => openSettings('appearance')}>
            <Icon name="sliders" />Settings
          </button>
          {user.admin && <a role="menuitem" class={styles.menuItem} href="/admin/accounts" onClick={() => setOpen(false)}>
            <Icon name="shield" />Admin
          </a>}
          <div class={styles.separator} />
          <button type="button" role="menuitem" class={`${styles.menuItem} ${styles.danger}`} disabled={busy} onClick={signOut}>
            <Icon name="logout" />Log out
          </button>
          {error && <Notice compact tone="error">{error}</Notice>}
        </div>
      )}
    </div>
  )
}
