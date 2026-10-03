import { createPortal } from 'preact/compat'
import type { ComponentChildren } from 'preact'
import { useEffect, useId, useRef } from 'preact/hooks'
import styles from './Modal.module.css'

const openModals: string[] = []

export function Modal({
  title,
  onClose,
  children,
  compact = false,
}: {
  title: string
  onClose: () => void
  children: ComponentChildren
  compact?: boolean
}) {
  const id = useId()
  const dialog = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  close.current = onClose
  useEffect(() => {
    openModals.push(id)
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    dialog.current?.focus()
    const keydown = (e: KeyboardEvent) => {
      if (openModals.at(-1) !== id) return
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        close.current()
      }
      if (e.key === 'Tab') {
        const elements = dialog.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
        )
        if (!elements?.length) {
          e.preventDefault()
          return
        }
        const first = elements[0],
          last = elements[elements.length - 1]
        if (
          e.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === dialog.current)
        ) {
          e.preventDefault()
          last.focus()
        } else if (
          !e.shiftKey &&
          (document.activeElement === last ||
            document.activeElement === dialog.current)
        ) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', keydown)
    return () => {
      openModals.splice(openModals.indexOf(id), 1)
      document.removeEventListener('keydown', keydown)
      previous?.focus()
    }
  }, [])
  return createPortal(
    <div
      class={styles.backdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        class={`${styles.modal} ${compact ? styles.compact : ''}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
        tabIndex={-1}
        ref={dialog}
      >
        <div class={styles.title}>
          <div id={id}>{title}</div>
          <button
            class={styles.close}
            type="button"
            onClick={onClose}
            aria-label="Close"
          >
            X
          </button>
        </div>
        {children}
      </div>
    </div>,
    document.body,
  )
}
