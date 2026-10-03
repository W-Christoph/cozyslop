import type { JSX } from 'preact'
import styles from './IconButton.module.css'

type Props = JSX.IntrinsicElements['button'] & { icon: string; label: string; active?: boolean }
export function IconButton({ icon, label, active = false, class: className, ...props }: Props) {
  return <button type="button" {...props} aria-label={label} title={props.title ?? label}
    class={`${styles.button} ${active ? styles.active : ''} ${className ?? ''}`}>
    <img src={`/svg/${icon}.svg`} alt="" />
  </button>
}
