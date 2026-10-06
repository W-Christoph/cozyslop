import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useRoomStore } from '../room/RoomContext'
import type { ChatMessage } from '../../room/protocol'
import { MentionPopup } from './MentionPopup'
import { useMentionSuggestions } from './useMentionSuggestions'
import styles from './MessageEditor.module.css'

export function MessageEditor({ message, onClose }: { message: ChatMessage; onClose: () => void }) {
  const store = useRoomStore()
  const [text, setText] = useState(message.body)
  const input = useRef<HTMLTextAreaElement>(null)
  const mentions = useMentionSuggestions(input, text, setText)
  useLayoutEffect(() => { input.current?.focus() }, [])
  useLayoutEffect(() => {
    if (!input.current) return
    input.current.style.height = '0px'
    input.current.style.height = `${input.current.scrollHeight}px`
  }, [text])
  function save() {
    if (!text.trim()) return
    store.editChat(message.id, text)
    mentions.close()
    onClose()
  }
  return <div class={styles.editor}>
    <div class={styles.wrapper}>
      <textarea {...mentions.inputProps} ref={input} aria-label="Edit message" value={text} maxlength={store.self.value?.anonymous ? 250 : undefined}
        onBlur={mentions.close}
        onInput={(e) => { setText(e.currentTarget.value); mentions.syncCaret() }}
        onKeyDown={(e) => {
          if (e.isComposing) return
          if (mentions.onKeyDown(e)) return
          if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); save() }
          else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); onClose() }
        }} />
    </div>
    <MentionPopup suggestions={mentions} />
    <p class={styles.hint}>Esc to <button type="button" onClick={onClose}>cancel</button>, Enter to <button type="button" onClick={save}>send</button></p>
  </div>
}
