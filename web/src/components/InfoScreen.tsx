import type { ComponentChildren } from 'preact'
import styles from './InfoScreen.module.css'

export function InfoScreen({
  message,
  submessage,
  children,
}: {
  message: string
  submessage?: string
  children?: ComponentChildren
}) {
  return (
    <main class={styles.screen}>
      <div class={styles.center}>
        <div class={styles.message}>{message}</div>
        {submessage && <div class={styles.submessage}>{submessage}</div>}
        {children && <div class={styles.actions}>{children}</div>}
      </div>
    </main>
  )
}
