import type { RefObject } from 'preact'
import { useEffect, useRef, useState } from 'preact/hooks'
import { useRoomStore } from './RoomContext'
import { useMobileKeyboard } from './useMobileKeyboard'
import { useDesktopPaste } from './useDesktopPaste'
import styles from './MobileRemoteControls.module.css'

export function MobileRemoteControls({ pointer }: { pointer: RefObject<{ x: number; y: number }> }) {
  const store = useRoomStore()
  const textarea = useRef<HTMLTextAreaElement>(null)
  const held = useRef(new Set<number>())
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const { requestPaste, pending, dialog } = useDesktopPaste()
  useMobileKeyboard(textarea, requestPaste, pending)
  useEffect(() => () => {
    held.current.forEach((button) => store.neko.buttonUp(button))
    held.current.clear()
  }, [store])
  const down = (button: number) => {
    if (!store.isHost.value) return
    const position = pointer.current
    if (position) store.neko.move(Math.round(position.x), Math.round(position.y))
    held.current.add(button)
    store.neko.buttonDown(button)
  }
  const up = (button: number) => {
    if (!held.current.delete(button)) return
    store.neko.buttonUp(button)
  }
  return (
    <><div class={styles.controls} aria-label="Mobile remote controls">
      <div class={styles.keyboard}>
        <button aria-label="Toggle remote keyboard" aria-pressed={keyboardOpen} onClick={() => {
          if (document.activeElement === textarea.current) textarea.current?.blur()
          else textarea.current?.focus()
        }}><img src="/svg/keyboard.svg" alt="" /></button>
        <textarea ref={textarea} class={styles.input} aria-label="Remote keyboard"
          autocomplete="off" autocapitalize="off" autoCorrect="off" spellcheck={false}
          onFocus={() => setKeyboardOpen(true)} onBlur={() => setKeyboardOpen(false)} />
      </div>
      {([{ button: 1, icon: 'left-click', label: 'Left click' }, { button: 3, icon: 'right-click', label: 'Right click' }]).map(({ button, icon, label }) => (
        <button key={button} aria-label={label} onPointerDown={(e) => {
          e.preventDefault()
          e.currentTarget.setPointerCapture(e.pointerId)
          down(button)
        }} onPointerUp={() => up(button)} onPointerCancel={() => up(button)} onLostPointerCapture={() => up(button)}
          onKeyDown={(e) => { if (!e.repeat && (e.key === ' ' || e.key === 'Enter')) { e.preventDefault(); down(button) } }}
          onKeyUp={(e) => { if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); up(button) } }} onBlur={() => up(button)}>
          <img src={`/svg/${icon}.svg`} alt="" />
        </button>
      ))}
      <button class={styles.scroll} aria-label="Scroll up" onClick={() => { if (store.isHost.value) store.neko.scroll(0, 1) }}><img src="/svg/arrow-up.svg" alt="" /></button>
      <button class={styles.scroll} aria-label="Scroll down" onClick={() => { if (store.isHost.value) store.neko.scroll(0, -1) }}><img src="/svg/arrow-down.svg" alt="" /></button>
    </div>{dialog}</>
  )
}
