import type { ComponentChildren } from 'preact'
import { useId } from 'preact/hooks'
import { Switch } from './Switch'
import formStyles from './Form.module.css'
import styles from './Section.module.css'

// A titled block of settings; blocks are separated by a line.
export function Section({ title, description, actions, children }: {
  title?: string
  description?: ComponentChildren
  actions?: ComponentChildren // beside the title
  children: ComponentChildren
}) {
  return (
    <section class={styles.section}>
      {(title || description || actions) && <div class={styles.head}>
        <div class={styles.headText}>
          {title && <h3 class={styles.title}>{title}</h3>}
          {description && <p class={styles.description}>{description}</p>}
        </div>
        {actions && <div class={styles.actions}>{actions}</div>}
      </div>}
      {children}
    </section>
  )
}

// A small heading inside a section.
export function SectionLabel({ children }: { children: ComponentChildren }) {
  return <div class={formStyles.label}>{children}</div>
}

// One setting: what it is on the left, its control on the right.
export function SettingRow({ title, description, htmlFor, children }: {
  title: string
  description?: ComponentChildren
  htmlFor?: string
  children: ComponentChildren
}) {
  return (
    <div class={styles.row}>
      <div class={styles.rowText}>
        {htmlFor ? <label class={`${styles.rowTitle} ${styles.clickable}`} for={htmlFor}>{title}</label> : <div class={styles.rowTitle}>{title}</div>}
        {description && <div class={styles.rowDescription}>{description}</div>}
      </div>
      <div class={styles.control}>{children}</div>
    </div>
  )
}

export function ToggleRow({ title, description, checked, onChange, disabled }: {
  title: string
  description?: ComponentChildren
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
}) {
  const id = useId()
  return (
    <SettingRow title={title} description={description} htmlFor={id}>
      <Switch id={id} checked={checked} onChange={onChange} disabled={disabled} />
    </SettingRow>
  )
}
