import { PermissionTable } from '../../components/admin/PermissionTable'
import { Section } from '../../components/ui/Section'

export function PermissionsTab() {
  return (
    <Section title="Permissions" description="What each account may do in each room, beyond the room's defaults. The form above the table adds a new one.">
      <PermissionTable />
    </Section>
  )
}
