import { me } from '../app/state'
import { ButtonLink } from '../components/Button'
import { PageLayout } from '../components/PageLayout'
import { WatchlogView } from '../components/watchlog/WatchlogView'
import { Notice } from '../components/ui/Notice'
import styles from './WatchlogPage.module.css'

export function WatchlogPage() {
  const user = me.value
  return (
    <div class={styles.page}>
      <PageLayout wide title="Watchlog" subtitle="Who has watched how far, and what you can watch together.">
        {user ? <WatchlogView user={user} /> : <>
          <Notice>Log in to see the watchlog.</Notice>
          <div class={styles.login}><ButtonLink variant="primary" href="/login">Log in</ButtonLink></div>
        </>}
      </PageLayout>
    </div>
  )
}
