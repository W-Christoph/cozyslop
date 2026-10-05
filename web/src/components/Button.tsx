import type { ComponentChildren, JSX } from 'preact'
import { Icon, type IconName } from './ui/Icon'
import styles from './Button.module.css'

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-outline' | 'danger-ghost'
interface Look {
  variant?: ButtonVariant
  size?: 'sm' | 'md' | 'lg'
  icon?: IconName
  block?: boolean
}

// bare: an icon without text.
function look({ variant, size = 'md', block }: Look, className?: string, bare = false) {
  const kind = variant === 'danger-outline' ? 'dangerOutline' : variant === 'danger-ghost' ? 'dangerGhost' : variant ?? 'secondary'
  return `${styles.button} ${styles[kind]} ${styles[size]} ${block ? styles.block : ''} ${bare ? styles.bare : ''} ${className ?? ''}`
}

type Props = Omit<JSX.IntrinsicElements['button'], 'size' | 'icon'> & Look
export function Button({ variant, size, icon, block, class: className, type = 'button', children, ...props }: Props) {
  return (
    <button {...props} type={type} class={look({ variant, size, block }, className as string, !!icon && children == null)}>
      {icon && <Icon name={icon} size={16} />}
      {children}
    </button>
  )
}

// A link that looks like a button.
export function ButtonLink({ variant, size, icon, block, class: className, children, ...props }: Look & {
  href: string
  download?: string
  title?: string
  'aria-label'?: string
  class?: string
  children?: ComponentChildren
}) {
  return (
    <a {...props} class={look({ variant, size, block }, className, !!icon && children == null)}>
      {icon && <Icon name={icon} size={16} />}
      {children}
    </a>
  )
}
