import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useRoomStore } from '../room/RoomContext'
import type { ChatMessage } from '../../room/protocol'
import styles from './MessageEditor.module.css'

export function MessageEditor({ message, onClose }: { message: ChatMessage; onClose: () => void }) {
  const store = useRoomStore()
  const [text, setText] = useState(message.body)
  const input = useRef<HTMLTextAreaElement>(null)
  useLayoutEffect(() => { input.current?.focus() }, [])
  useLayoutEffect(() => {
    if (!input.current) return
    input.current.style.height = '0px'
    input.current.style.height = `${input.current.scrollHeight}px`
  }, [text])
  function save() {
    if (!text.trim()) return
    store.editChat(message.id, text)
    onClose()
  }
  return <div class={styles.editor}>
    <div class={styles.wrapper}>
      <textarea ref={input} aria-label="Edit message" value={text} maxLength={store.self.value?.anonymous ? 250 : undefined}
        onInput={(e) => setText(e.currentTarget.value)}
        onKeyDown={(e) => {
          if (e.isComposing) return
          if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); save() }
          else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
        }} />
    </div>
    <p class={styles.hint}>Esc to <button type="button" onClick={onClose}>cancel</button>, Enter to <button type="button" onClick={save}>send</button></p>
  </div>
}
