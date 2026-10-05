import { useLayoutEffect, useRef } from 'preact/hooks'

const openDialogs: string[] = []

// Shared by framed dialogs and the bare chat media preview.
export function useDialogFocus(id: string, onClose: () => void) {
  const dialog = useRef<HTMLDivElement>(null)
  const close = useRef(onClose)
  close.current = onClose
  useLayoutEffect(() => {
    openDialogs.push(id)
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const elements = () => {
      const all = [...(dialog.current?.querySelectorAll<HTMLElement>(
        'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"], video[controls]',
      ) ?? [])].filter((element) => element.offsetParent !== null && element.tabIndex !== -1)
      return all.filter((element) => element.getAttribute('type') !== 'radio' ||
        !all.some((other) => other.getAttribute('name') === element.getAttribute('name') && other.getAttribute('type') === 'radio' && (other as HTMLInputElement).checked) ||
        (element as HTMLInputElement).checked)
    }
    const first = elements()[0]
    if (first) first.focus()
    else dialog.current?.focus()
    const keydown = (e: KeyboardEvent) => {
      if (openDialogs.at(-1) !== id) return
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        close.current()
      }
      if (e.key === 'Tab') {
        const controls = elements()
        if (!controls.length) {
          e.preventDefault()
          return
        }
        const first = controls[0], last = controls[controls.length - 1]
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
  return dialog
}
