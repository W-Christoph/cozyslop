import styles from './Spinner.module.css'

export function LoadingIndicator({ large = false }: { large?: boolean }) {
  return <span class={`${styles.spinner} ${large ? styles.large : ''}`} aria-hidden="true" />
}

export function Spinner({ label, inline = false }: { label: string; inline?: boolean }) {
  return <div class={`${styles.loading} ${inline ? styles.inline : ''}`} role="status"><LoadingIndicator />{label}</div>
}
