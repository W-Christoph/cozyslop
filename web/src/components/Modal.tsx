import type { ComponentChildren } from 'preact'
import { useId } from 'preact/hooks'
import { Dialog } from './ui/Dialog'
import { CloseButton } from './ui/CloseButton'
import styles from './Modal.module.css'

export function Modal({
  title,
  onClose,
  children,
  footer,
  size = 'md',
}: {
  title: string
  onClose: () => void
  children: ComponentChildren
  footer?: ComponentChildren // buttons, right-aligned under the content
  size?: 'sm' | 'md' | 'lg' | 'xl'
}) {
  const id = useId()
  return (
    <Dialog id={id} labelledBy={id} onClose={onClose} class={`${styles.modal} ${styles[size]}`}>
      <header class={styles.header}>
        <h2 id={id} class={styles.title}>{title}</h2>
        <CloseButton onClick={onClose} />
      </header>
      <div class={styles.body}>{children}</div>
      {footer && <footer class={styles.footer}>{footer}</footer>}
    </Dialog>
  )
}
