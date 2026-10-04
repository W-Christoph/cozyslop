import type { RefObject } from 'preact'
import { useLayoutEffect } from 'preact/hooks'
import { useRoomStore } from './RoomContext'

// X11 keysyms of the keys a mobile keyboard reports as keys.
const KEYSYMS = { Backspace: 0xff08, Enter: 0xff0d, Delete: 0xffff } as const
type Key = keyof typeof KEYSYMS
const isKey = (name: string): name is Key => Object.hasOwn(KEYSYMS, name)

// A mobile keyboard reports text, not key presses: characters arrive without
// the modifiers a physical key needs (a lone "A" pressed as a key comes out
// as "a"). So all text is inserted as text, through neko's paste, and only
// Enter and the delete keys are sent as keys.
export function useMobileKeyboard(textarea: RefObject<HTMLTextAreaElement | null>) {
  const store = useRoomStore()
  useLayoutEffect(() => {
    const el = textarea.current
    if (!el) return
    const key = (name: Key) => {
      if (!store.isHost.value) return
      store.neko.keyDown(KEYSYMS[name])
      store.neko.keyUp(KEYSYMS[name])
    }
    const text = (value: string) => {
      if (store.isHost.value && value) store.neko.paste(value)
    }
    let composing = false
    let composition = ''
    let clearComposition: number | undefined
    const reset = () => { el.value = ' '; el.setSelectionRange(1, 1) }
    const keydown = (e: KeyboardEvent) => {
      if (!store.isHost.value || composing || e.isComposing || e.keyCode === 229) return
      const printable = Array.from(e.key).length === 1
      if (!printable && !isKey(e.key)) return
      e.preventDefault()
      if (isKey(e.key)) key(e.key)
      else text(e.key)
      reset()
    }
    const beforeinput = (e: InputEvent) => {
      if (!store.isHost.value || composing || e.isComposing || e.inputType === 'insertCompositionText') return
      if (!e.cancelable) return // input handles browsers with non-cancelable beforeinput
      e.preventDefault()
      if (e.data && e.data === composition) return
      switch (e.inputType) {
        case 'deleteContentBackward': key('Backspace'); break
        case 'deleteContentForward': key('Delete'); break
        case 'insertLineBreak':
        case 'insertParagraph': key('Enter'); break
        default: text(e.data ?? '')
      }
      reset()
    }
    const input = (e: InputEvent) => {
      if (composing || e.isComposing) return
      if (!composition || e.data !== composition) {
        if (e.inputType === 'deleteContentBackward') key('Backspace')
        else if (e.inputType === 'deleteContentForward') key('Delete')
        else if (e.inputType === 'insertLineBreak') key('Enter')
        else text(e.data ?? '')
      }
      reset()
    }
    const compositionstart = () => { composing = true }
    const compositionend = (e: CompositionEvent) => {
      composing = false
      composition = e.data
      text(e.data)
      reset()
      window.clearTimeout(clearComposition)
      clearComposition = window.setTimeout(() => { composition = '' }, 0)
    }
    const paste = (e: ClipboardEvent) => {
      e.preventDefault()
      text(e.clipboardData?.getData('text/plain') ?? '')
      reset()
    }
    reset()
    el.addEventListener('keydown', keydown)
    el.addEventListener('beforeinput', beforeinput)
    el.addEventListener('input', input)
    el.addEventListener('compositionstart', compositionstart)
    el.addEventListener('compositionend', compositionend)
    el.addEventListener('paste', paste)
    return () => {
      window.clearTimeout(clearComposition)
      el.removeEventListener('keydown', keydown)
      el.removeEventListener('beforeinput', beforeinput)
      el.removeEventListener('input', input)
      el.removeEventListener('compositionstart', compositionstart)
      el.removeEventListener('compositionend', compositionend)
      el.removeEventListener('paste', paste)
    }
  }, [store, textarea])
}
