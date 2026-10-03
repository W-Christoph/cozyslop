import { useEffect } from 'preact/hooks'
import { useLocation, useRoute } from 'preact-iso'
import { me } from '../../app/state'
import { AccountsTab } from './AccountsTab'
import { ServerSettingsTab } from './ServerSettingsTab'
import { PermissionsTab } from './PermissionsTab'
import { InvitesTab } from './InvitesTab'
import styles from './AdminPage.module.css'

export function AdminPage() {
  const { params } = useRoute()
  const { route } = useLocation()
  const allowed = !!me.value?.admin
  const tab = params.tab || 'accounts'
  useEffect(() => {
    if (!allowed) route('/')
  }, [allowed])
  if (!allowed) return null
  return (
    <main class={styles.background}>
      <nav class={styles.nav} aria-label="Admin navigation">
        {['settings', 'accounts', 'permissions', 'invites'].map((name) => (
          <a
            key={name}
            href={`/admin/${name}`}
            class={tab === name ? styles.active : undefined}
          >
            {name[0].toUpperCase() + name.slice(1)}
          </a>
        ))}
      </nav>
      <div class={styles.content}>
        {tab === 'settings' ? (
          <ServerSettingsTab />
        ) : tab === 'permissions' ? (
          <PermissionsTab />
        ) : tab === 'invites' ? (
          <InvitesTab />
        ) : (
          <AccountsTab />
        )}
      </div>
    </main>
  )
}
