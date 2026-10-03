import styles from './InfoScreen.module.css'

// A centered full-page message, e.g. while connecting.
export function InfoScreen({ message, submessage }: { message: string; submessage?: string }) {
  return (
    <div class={styles.screen}>
      <div class={styles.message}>{message}</div>
      {submessage && <div class={styles.submessage}>{submessage}</div>}
    </div>
  )
}
