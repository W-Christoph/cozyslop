import type { ComponentChildren, JSX } from 'preact'
import { Icon } from './Icon'
import styles from './Field.module.css'

// A labelled form control, label above.
export function Field({ label, hint, children, class: className }: {
  label: string
  hint?: ComponentChildren
  children: ComponentChildren
  class?: string
}) {
  return (
    <label class={`${styles.field} ${className ?? ''}`}>
      <span class={styles.label}>{label}</span>
      {children}
      {hint && <span class={styles.hint}>{hint}</span>}
    </label>
  )
}

// compact: for tables.
export function Input({ class: className, compact, ...props }: JSX.IntrinsicElements['input'] & { compact?: boolean }) {
  return <input {...props} class={`${styles.input} ${compact ? styles.compact : ''} ${className ?? ''}`} />
}

export function Textarea({ class: className, ...props }: JSX.IntrinsicElements['textarea']) {
  return <textarea {...props} class={`${styles.input} ${styles.textarea} ${className ?? ''}`} />
}

export function Select({ class: className, compact, children, ...props }: JSX.IntrinsicElements['select'] & { compact?: boolean }) {
  return (
    <span class={`${styles.select} ${compact ? styles.compact : ''} ${className ?? ''}`}>
      <select {...props}>{children}</select>
      <Icon name="chevronDown" size={16} />
    </span>
  )
}

type CheckboxProps = Pick<JSX.IntrinsicElements['input'], 'checked' | 'disabled' | 'onChange' | 'id' | 'aria-label'> & { class?: string }
export function Checkbox({ class: className, ...props }: CheckboxProps) {
  return <input {...props} type="checkbox" class={`${styles.checkbox} ${className ?? ''}`} />
}
