import styles from './Switch.module.css'

export function Switch({ checked, onChange, disabled, label, id }: {
  checked: boolean
  onChange: (checked: boolean) => void
  disabled?: boolean
  label?: string // for switches without a visible label
  id?: string
}) {
  return (
    <button type="button" role="switch" id={id} class={styles.switch} aria-checked={checked} aria-label={label}
      disabled={disabled} onClick={() => onChange(!checked)}>
      <span class={styles.thumb} />
    </button>
  )
}
