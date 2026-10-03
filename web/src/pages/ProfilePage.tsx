import { me } from '../app/state'
import { PageLayout } from '../components/PageLayout'
import { ProfileEditor } from '../components/profile/ProfileEditor'
import { ChangePassword } from '../components/profile/ChangePassword'

export function ProfilePage() {
  return (
    <PageLayout panel={false}>
      {me.value ? (
        <>
          <ProfileEditor />
          <ChangePassword />
        </>
      ) : (
        <p>
          Please <a href="/login">log in</a> to edit your profile.
        </p>
      )}
    </PageLayout>
  )
}
