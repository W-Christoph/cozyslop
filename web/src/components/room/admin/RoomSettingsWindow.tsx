import { useState } from 'preact/hooks'
import { InviteList } from '../../admin/InviteList'
import { InviteModal } from '../../admin/InviteModal'
import { PermissionTable } from '../../admin/PermissionTable'
import { Button } from '../../Button'
import { Section } from '../../ui/Section'
import { SettingsWindow, type NavGroup } from '../../ui/SettingsLayout'
import { useRoomStore } from '../RoomContext'
import { AnonymousBans } from './AnonymousBans'
import { CurrentRoomUsers } from './CurrentRoomUsers'
import { RoomAccessSettings } from './RoomAccessSettings'
import { RoomTools } from './RoomTools'
import { StreamSettings } from './StreamSettings'
import styles from './RoomSettingsWindow.module.css'

type SectionId = 'access' | 'stream' | 'people' | 'permissions' | 'bans' | 'invites' | 'tools'
const nav: NavGroup[] = [
  {
    label: 'Room',
    items: [
      { id: 'access', label: 'Access', icon: 'lock' },
      { id: 'stream', label: 'Stream', icon: 'monitor' },
      { id: 'tools', label: 'Tools', icon: 'zap' },
    ],
  },
  {
    label: 'People',
    items: [
      { id: 'people', label: 'In the room', icon: 'users' },
      { id: 'permissions', label: 'Permissions', icon: 'key' },
      { id: 'invites', label: 'Invites', icon: 'ticket' },
      { id: 'bans', label: 'Anonymous bans', icon: 'ban' },
    ],
  },
]
const wide: SectionId[] = ['people', 'permissions', 'invites']

// Everything an admin can change about the room they are in.
export function RoomSettingsWindow({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const [section, setSection] = useState<SectionId>('access')
  const [invite, setInvite] = useState(false)
  // A new invite is listed once the list is read again.
  const [invites, setInvites] = useState(0)
  if (!store.rights.value.admin) return null
  return (
    <SettingsWindow left nav={nav} current={section} wide={wide.includes(section)} onClose={onClose}
      onSelect={(id) => setSection(id as SectionId)}
      navHeader={<div class={styles.room}><span>Room settings</span><strong>{store.settings.value?.name ?? store.room}</strong></div>}>
      {section === 'access' && <RoomAccessSettings />}
      {section === 'stream' && <StreamSettings />}
      {section === 'tools' && <RoomTools />}
      {section === 'people' && (
        <Section title="People in the room" description="Who is watching right now and what they may do here. Changes for accounts are kept; an anonymous visitor keeps theirs until they leave.">
          <CurrentRoomUsers />
        </Section>
      )}
      {section === 'permissions' && (
        <Section title="Account permissions" description="Every account with its own permissions in this room, present or not. The first row adds a new one.">
          <PermissionTable room={store.room} />
        </Section>
      )}
      {section === 'invites' && (
        <Section title="Invite links" description="Links that let people into this room."
          actions={<Button accent icon="plus" onClick={() => setInvite(true)}>Create invite</Button>}>
          <InviteList key={invites} room={store.room} />
        </Section>
      )}
      {section === 'bans' && (
        <Section title="Banned addresses" description="Visitors without an account are banned by address. Bans of accounts are under Permissions.">
          <AnonymousBans />
        </Section>
      )}
      {invite && <InviteModal room={store.room} onClose={() => { setInvite(false); setInvites((value) => value + 1) }} />}
    </SettingsWindow>
  )
}
