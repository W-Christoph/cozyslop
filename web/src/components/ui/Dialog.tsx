import { createPortal } from 'preact/compat'
import type { ComponentChildren } from 'preact'
import { useState } from 'preact/hooks'
import { useDialogFocus } from './useDialogFocus'
import styles from './Dialog.module.css'

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
  const dialog = useDialogFocus(id, onClose)
  // Fullscreen hides DOM outside its element, including body-level portals.
  // Chosen once: moving an open dialog to another container would remount
  // it and lose what was typed.
  const [container] = useState(() => document.fullscreenElement ?? document.body)
  return createPortal(
    <div class={styles.backdrop} onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <div data-ui class={`${styles.dialog} ${className ?? ''}`} role="dialog" aria-modal="true" aria-labelledby={labelledBy}
        tabIndex={-1} ref={dialog}>
        {children}
      </div>
    </div>,
    container,
  )
}
