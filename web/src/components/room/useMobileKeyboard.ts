import type { RefObject } from 'preact'
import { useLayoutEffect } from 'preact/hooks'
import GuacamoleKeyboard from '../../neko/guacamole-keyboard.js'
import { useRoomStore } from './RoomContext'

// Use the same Guacamole translator as desktop input. A detached target
// avoids sending both the native mobile event and its beforeinput fallback.
export function useMobileKeyboard(textarea: RefObject<HTMLTextAreaElement | null>) {
  const store = useRoomStore()
  useLayoutEffect(() => {
    const el = textarea.current
    if (!el) return
    const target = document.createElement('div')
    const keyboard = new GuacamoleKeyboard(target)
    keyboard.onkeydown = (keysym) => {
      if (store.isHost.value) store.neko.keyDown(keysym)
      return false
    }
    keyboard.onkeyup = (keysym) => {
      if (store.isHost.value) store.neko.keyUp(keysym)
    }
    const key = (value: string) => {
      if (!store.isHost.value) return
      const special: Record<string, number> = { Backspace: 8, Enter: 13, Delete: 46 }
      const keyCode = special[value] ?? value.toUpperCase().charCodeAt(0)
      target.dispatchEvent(new KeyboardEvent('keydown', { key: value, keyCode, cancelable: true }))
      if (Array.from(value).length === 1) {
        target.dispatchEvent(new KeyboardEvent('keypress', {
          key: value, keyCode: value.codePointAt(0), charCode: value.codePointAt(0), cancelable: true,
        }))
      }
      target.dispatchEvent(new KeyboardEvent('keyup', { key: value, keyCode, cancelable: true }))
    }
    const text = (value: string) => {
      if (!store.isHost.value || !value) return
      if (Array.from(value).length === 1) key(value)
      else store.neko.paste(value)
    }
    let composing = false
    let composition = ''
    let clearComposition: number | undefined
    const reset = () => { el.value = ' '; el.setSelectionRange(1, 1) }
    const keydown = (e: KeyboardEvent) => {
      if (!store.isHost.value || composing || e.isComposing || e.keyCode === 229) return
      if (Array.from(e.key).length === 1 || ['Backspace', 'Enter', 'Delete'].includes(e.key)) {
        e.preventDefault()
        key(e.key)
        reset()
      }
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
      if (store.isHost.value && e.data) store.neko.paste(e.data)
      reset()
      window.clearTimeout(clearComposition)
      clearComposition = window.setTimeout(() => { composition = '' }, 0)
    }
    const paste = (e: ClipboardEvent) => {
      e.preventDefault()
      if (store.isHost.value) store.neko.paste(e.clipboardData?.getData('text/plain') ?? '')
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
      keyboard.reset()
      el.removeEventListener('keydown', keydown)
      el.removeEventListener('beforeinput', beforeinput)
      el.removeEventListener('input', input)
      el.removeEventListener('compositionstart', compositionstart)
      el.removeEventListener('compositionend', compositionend)
      el.removeEventListener('paste', paste)
    }
  }, [store, textarea])
}
