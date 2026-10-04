import type { ComponentChildren } from 'preact'
import styles from './PageLayout.module.css'

// A page under the site header: a heading and its content in a centred column.
export function PageLayout({
  title,
  subtitle,
  children,
  narrow = false,
}: {
  title?: string
  subtitle?: ComponentChildren
  children: ComponentChildren
  narrow?: boolean // text pages
}) {
  return (
    <main class={`${styles.page} ${narrow ? styles.narrow : ''}`}>
      {title && (
        <div class={styles.head}>
          <div class={styles.headText}>
            {title && <h1 class={styles.title}>{title}</h1>}
            {subtitle && <p class={styles.subtitle}>{subtitle}</p>}
          </div>
        </div>
      )}
      {children}
    </main>
  )
}

// A single card in the middle of the page: logging in, registering.
export function AuthLayout({ title, subtitle, children, footer }: {
  title: string
  subtitle?: ComponentChildren
  children: ComponentChildren
  footer?: ComponentChildren
}) {
  return (
    <main class={styles.auth}>
      <div class={styles.card}>
        <img class={styles.logo} src="/png/favicon.png" alt="" width={56} height={56} />
        <h1 class={styles.authTitle}>{title}</h1>
        {subtitle && <p class={styles.authSubtitle}>{subtitle}</p>}
        <div class={styles.authBody}>{children}</div>
      </div>
      {footer && <p class={styles.authFooter}>{footer}</p>}
    </main>
  )
}
