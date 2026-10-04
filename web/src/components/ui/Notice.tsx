import type { ComponentChildren } from 'preact'
import { Icon } from './Icon'
import styles from './Notice.module.css'

// An inline message: errors are announced at once, the rest politely.
export function Notice({ tone = 'info', children }: { tone?: 'error' | 'success' | 'info'; children: ComponentChildren }) {
  return (
    <p class={`${styles.notice} ${styles[tone]}`} role={tone === 'error' ? 'alert' : 'status'}>
      <Icon name={tone === 'error' ? 'alert' : tone === 'success' ? 'check' : 'info'} size={16} />
      <span>{children}</span>
    </p>
  )
}
