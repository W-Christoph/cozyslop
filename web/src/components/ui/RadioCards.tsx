import type { ComponentChildren } from 'preact'
import styles from './RadioCards.module.css'

export interface RadioOption<T extends string> {
  value: T
  label: string
  description?: string
  preview?: ComponentChildren // drawn above the label
}

// A single choice shown as cards.
export function RadioCards<T extends string>({ name, label, value, options, onChange, columns, mobileColumns = 1, disabled }: {
  name: string
  label: string
  value: T
  options: RadioOption<T>[]
  onChange: (value: T) => void
  columns?: number
  mobileColumns?: number // on phones, when columns is set
  disabled?: boolean
}) {
  return (
    <div class={`${styles.cards} ${columns ? styles.fixed : ''}`} role="radiogroup" aria-label={label}
      style={columns ? { '--columns': columns, '--mobile-columns': mobileColumns } : undefined}>
      {options.map((option) => (
        <label data-focus-ring key={option.value} class={`${styles.card} ${option.value === value ? styles.selected : ''}`}>
          <input data-focus-proxy type="radio" name={name} value={option.value} checked={option.value === value} disabled={disabled}
            onChange={() => onChange(option.value)} />
          {option.preview && <div class={styles.preview}>{option.preview}</div>}
          <div class={styles.text}>
            <span class={styles.dot} />
            <span>
              <span class={styles.label}>{option.label}</span>
              {option.description && <span class={styles.description}>{option.description}</span>}
            </span>
          </div>
        </label>
      ))}
    </div>
  )
}
