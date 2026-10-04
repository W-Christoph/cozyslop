import { InviteList } from '../../components/admin/InviteList'
import { Section } from '../../components/ui/Section'

export function InvitesTab() {
  return (
    <Section title="Invites" description="Invite and access links of all rooms. New ones are created from a room: Invite on the rooms page, or the room's settings.">
      <InviteList />
    </Section>
  )
}
