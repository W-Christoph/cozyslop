import type { ComponentChildren } from 'preact'
import { useId } from 'preact/hooks'
import { Dialog } from './ui/Dialog'
import { Icon } from './ui/Icon'
import styles from './Modal.module.css'

export function Modal({
  title,
  onClose,
  children,
  footer,
  compact = false,
  size,
}: {
  title: string
  onClose: () => void
  children: ComponentChildren
  footer?: ComponentChildren // buttons, right-aligned under the content
  compact?: boolean // same as size="sm"
  size?: 'sm' | 'md' | 'lg' | 'xl'
}) {
  const id = useId()
  return (
    <Dialog id={id} labelledBy={id} onClose={onClose} class={`${styles.modal} ${styles[size ?? (compact ? 'sm' : 'md')]}`}>
      <header class={styles.header}>
        <h2 id={id} class={styles.title}>{title}</h2>
        <button class={styles.close} type="button" onClick={onClose} aria-label="Close">
          <Icon name="x" size={20} />
        </button>
      </header>
      <div class={styles.body}>{children}</div>
      {footer && <footer class={styles.footer}>{footer}</footer>}
    </Dialog>
  )
}
