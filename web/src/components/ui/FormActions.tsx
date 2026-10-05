import type { ComponentChildren } from 'preact'
import { Notice } from './Notice'
import styles from './FormActions.module.css'

export function FormActions({ error, message, children }: { error?: string; message?: string; children: ComponentChildren }) {
  return <div class={styles.actions}>
    {error && <Notice class={styles.feedback} tone="error">{error}</Notice>}
    {message && <Notice class={styles.feedback} tone="success">{message}</Notice>}
    {children}
  </div>
}
