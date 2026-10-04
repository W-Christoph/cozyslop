import type { ComponentChildren } from 'preact'
import { Icon, type IconName } from './Icon'
import styles from './Badge.module.css'

export type Tone = 'neutral' | 'accent' | 'success' | 'danger' | 'warning'
export function Badge({ tone = 'neutral', icon, children, title }: {
  tone?: Tone
  icon?: IconName
  children: ComponentChildren
  title?: string
}) {
  return <span class={`${styles.badge} ${styles[tone]}`} title={title}>{icon && <Icon name={icon} size={12} />}{children}</span>
}
