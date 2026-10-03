import type { ComponentChildren } from 'preact'
import styles from './PageLayout.module.css'

export function PageLayout({
  title,
  children,
  panel = true,
}: {
  title?: string
  children: ComponentChildren
  panel?: boolean
}) {
  return (
    <main class={styles.background}>
      <div class={panel ? styles.panel : styles.content}>
        {title && <div class={styles.title}>{title}</div>}
        {children}
      </div>
    </main>
  )
}
