import type { ComponentChildren } from 'preact'
import { Icon, type IconName } from './ui/Icon'
import { LoadingIndicator } from './ui/Spinner'
import styles from './InfoScreen.module.css'

// A whole-page message: loading, an error, the reason someone was removed.
export function InfoScreen({
  message,
  submessage,
  icon,
  busy = false,
  children,
}: {
  message: string
  submessage?: string
  icon?: IconName
  busy?: boolean // show a spinner instead of an icon
  children?: ComponentChildren
}) {
  return (
    <main data-ui class={styles.screen}>
      <div class={styles.card}>
        {busy ? <LoadingIndicator large /> : <div class={styles.icon}><Icon name={icon ?? 'info'} size={26} /></div>}
        <h1 class={styles.message}>{message}</h1>
        {submessage && <p class={styles.submessage}>{submessage}</p>}
        {children && <div class={styles.actions}>{children}</div>}
      </div>
    </main>
  )
}
