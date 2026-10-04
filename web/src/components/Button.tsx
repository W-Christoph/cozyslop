import type { ComponentChildren, JSX } from 'preact'
import { Icon, type IconName } from './ui/Icon'
import styles from './Button.module.css'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger'
interface Look {
  accent?: boolean // same as variant="primary"
  variant?: ButtonVariant
  size?: 'sm' | 'md' | 'lg'
  icon?: IconName
  block?: boolean
}

// bare: an icon without text.
function look({ accent, variant, size = 'md', block }: Look, className?: string, bare = false) {
  const kind = variant ?? (accent ? 'primary' : 'secondary')
  return `${styles.button} ${styles[kind]} ${styles[size]} ${block ? styles.block : ''} ${bare ? styles.bare : ''} ${className ?? ''}`
}

type Props = Omit<JSX.IntrinsicElements['button'], 'size' | 'icon'> & Look
export function Button({ accent, variant, size, icon, block, class: className, type = 'button', children, ...props }: Props) {
  return (
    <button {...props} type={type} class={look({ accent, variant, size, block }, className as string, !!icon && children == null)}>
      {icon && <Icon name={icon} size={16} />}
      {children}
    </button>
  )
}

// A link that looks like a button.
export function ButtonLink({ accent, variant, size, icon, block, class: className, children, ...props }: Look & {
  href: string
  download?: string
  class?: string
  children?: ComponentChildren
}) {
  return (
    <a {...props} class={look({ accent, variant, size, block }, className)}>
      {icon && <Icon name={icon} size={16} />}
      {children}
    </a>
  )
}
