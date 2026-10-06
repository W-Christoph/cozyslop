import { useEffect } from 'preact/hooks'
import { useLocation, useRoute } from 'preact-iso'
import { me } from '../../app/state'
import { SettingsLayout, type NavGroup } from '../../components/ui/SettingsLayout'
import { AccountsTab } from './AccountsTab'
import { ServerSettingsTab } from './ServerSettingsTab'
import { PermissionsTab } from './PermissionsTab'
import { InvitesTab } from './InvitesTab'
import { RoomsTab } from './RoomsTab'

const nav: NavGroup[] = [
  {
    label: 'People',
    items: [
      { id: 'accounts', label: 'Accounts', icon: 'users', href: '/admin/accounts' },
      { id: 'permissions', label: 'Permissions', icon: 'key', href: '/admin/permissions' },
      { id: 'invites', label: 'Invites', icon: 'ticket', href: '/admin/invites' },
    ],
  },
  { label: 'Server', items: [
    { id: 'rooms', label: 'Rooms', icon: 'monitor', href: '/admin/rooms' },
    { id: 'settings', label: 'Server settings', icon: 'server', href: '/admin/settings' },
  ] },
]
const tabs = ['accounts', 'permissions', 'invites', 'rooms', 'settings']

export function AdminPage() {
  const { params } = useRoute()
  const { route } = useLocation()
  const allowed = !!me.value?.admin
  const tab = tabs.includes(params.tab) ? params.tab : 'accounts'
  useEffect(() => {
    if (!allowed) route('/')
  }, [allowed])
  if (!allowed) return null
  return (
    <main>
      <SettingsLayout page left nav={nav} current={tab} wide={tab !== 'settings'}>
        {tab === 'settings' ? (
          <ServerSettingsTab />
        ) : tab === 'permissions' ? (
          <PermissionsTab />
        ) : tab === 'invites' ? (
          <InvitesTab />
        ) : tab === 'rooms' ? (
          <RoomsTab />
        ) : (
          <AccountsTab />
        )}
      </SettingsLayout>
    </main>
  )
}
