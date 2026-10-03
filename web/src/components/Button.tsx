import type { JSX } from 'preact'
import styles from './Button.module.css'

type Props = JSX.IntrinsicElements['button'] & { accent?: boolean }
export function Button({
  accent = false,
  class: className,
  type = 'button',
  ...props
}: Props) {
  return (
    <button
      {...props}
      type={type}
      class={`${styles.button} ${accent ? styles.accent : ''} ${className ?? ''}`}
    />
  )
}
