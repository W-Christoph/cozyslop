import { createPortal } from 'preact/compat'
import type { ComponentChildren } from 'preact'
import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import styles from './Dialog.module.css'

const openDialogs: string[] = []

// A window over the dimmed page: keeps the keyboard inside, closes on Escape
// and on a click outside, and gives the focus back afterwards. Modal and
// SettingsWindow put their frames into it.
export function Dialog({ id, labelledBy, onClose, class: className, children }: {
  id: string // unique among open dialogs; only the top one reacts to keys
  labelledBy: string
  onClose: () => void
  class?: string
  children: ComponentChildren
}) {
  const dialog = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  close.current = onClose
  // Fullscreen hides DOM outside its element, including body-level portals.
  // Chosen once: moving an open dialog to another container would remount
  // it and lose what was typed.
  const [container] = useState(() => document.fullscreenElement ?? document.body)
  useLayoutEffect(() => {
    openDialogs.push(id)
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
    dialog.current?.focus()
    const keydown = (e: KeyboardEvent) => {
      if (openDialogs.at(-1) !== id) return
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        close.current()
      }
      if (e.key === 'Tab') {
        const elements = [...(dialog.current?.querySelectorAll<HTMLElement>(
          'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
        ) ?? [])].filter((element) => element.offsetParent !== null)
        if (!elements.length) {
          e.preventDefault()
          return
        }
        const first = elements[0], last = elements[elements.length - 1]
        if (e.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && (document.activeElement === last || document.activeElement === dialog.current)) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', keydown)
    return () => {
      openDialogs.splice(openDialogs.indexOf(id), 1)
      document.removeEventListener('keydown', keydown)
      previous?.focus()
    }
  }, [])
  return createPortal(
    <div class={styles.backdrop} onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <div class={`${styles.dialog} ${className ?? ''}`} role="dialog" aria-modal="true" aria-labelledby={labelledBy}
        tabIndex={-1} ref={dialog}>
        {children}
      </div>
    </div>,
    container,
  )
}
