import type { ComponentChildren } from 'preact'
import { Icon, type IconName } from './Icon'
import styles from './EmptyState.module.css'

export function EmptyState({ icon, title, children }: { icon: IconName; title: string; children?: ComponentChildren }) {
  return (
    <div class={styles.empty}>
      <div class={styles.icon}><Icon name={icon} size={22} /></div>
      <div class={styles.title}>{title}</div>
      {children && <div class={styles.text}>{children}</div>}
    </div>
  )
}

export function Spinner({ label }: { label: string }) {
  return <div class={styles.loading} role="status"><span class={styles.spinner} />{label}</div>
}
