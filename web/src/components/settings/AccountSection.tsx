import { me } from '../../app/state'
import { ButtonLink } from '../Button'
import { ChangePassword } from '../profile/ChangePassword'
import { ProfileEditor } from '../profile/ProfileEditor'
import { EmptyState } from '../ui/EmptyState'
import { Section } from '../ui/Section'

export function AccountSection() {
  if (!me.value)
    return (
      <EmptyState icon="user" title="You are not logged in">
        <p>Log in to choose a nickname, a name colour and a profile picture.</p>
        <ButtonLink accent href="/login">Log in</ButtonLink>
      </EmptyState>
    )
  return <>
    <Section title="Profile" description="How you appear to others in rooms.">
      <ProfileEditor />
    </Section>
    <Section title="Password">
      <ChangePassword />
    </Section>
  </>
}
