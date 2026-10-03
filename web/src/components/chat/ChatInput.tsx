import type { RefObject } from 'preact'
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { me } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { TypingIndicator } from './TypingIndicator'
import { UploadControls } from './UploadControls'
import styles from './ChatInput.module.css'

export function ChatInput({ inputRef, onEdit }: { inputRef: RefObject<HTMLTextAreaElement | null>; onEdit: (id: number) => void }) {
  const store = useRoomStore()
  const [text, setText] = useState('')
  const [notice, setNotice] = useState('')
  const lastTyping = useRef(-Infinity)
  const fileReceiver = useRef<((file: File) => void) | null>(null)
  const connected = store.server.value === 'connected'
  const error = store.error.value
  const anonymous = store.self.value?.anonymous ?? !me.value
  useLayoutEffect(() => {
    const ta = inputRef.current
    if (!ta) return
    ta.style.height = '0px'
    ta.style.height = `${Math.min(18 * 5, ta.scrollHeight)}px`
  }, [text, inputRef])
  useEffect(() => {
    if (!connected || !error) { setNotice(''); return }
    setNotice(error)
    const timer = window.setTimeout(() => setNotice(''), 5000)
    return () => window.clearTimeout(timer)
  }, [error, connected])
  useEffect(() => () => store.setTyping(false), [store])
  function stopTyping() { store.setTyping(false) }
  function send() {
    if (!text.trim() || !connected) return
    store.sendChat(text.replace(/^\n+|\n+$/g, ''))
    setText('')
    stopTyping()
  }
  return <div class={styles.chatbox}>
    <div data-chat-input data-has-text={!!text} class={styles.uploader}>
      <div data-chat-input-wrapper class={styles.wrapper}>
        <textarea aria-label="Chat message" ref={inputRef} value={text} rows={1} maxLength={anonymous ? 250 : undefined}
          class={`${styles.textarea} ${!text && store.rights.value.image ? styles.withUploads : ''}`}
          onBlur={stopTyping}
          onInput={(e) => {
            const value = anonymous ? e.currentTarget.value.slice(0, 250) : e.currentTarget.value
            setText(value)
            if (!value) stopTyping()
            else if (Date.now() - lastTyping.current >= 1000) { store.setTyping(true); lastTyping.current = Date.now() }
          }}
          onKeyDown={(e) => {
            if (e.isComposing) return
            if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); send() }
            else if (e.key === 'ArrowUp' && !text) {
              const last = [...store.chat.value].reverse().find((m) => m.author === store.selfKey.value && m.type === 'text' && m.id > 0 && !m.deleted)
              if (last) { e.preventDefault(); onEdit(last.id) }
            }
          }}
          onPaste={(e) => {
            if (!store.rights.value.image) return
            for (const item of e.clipboardData?.items ?? []) {
              if (!item.type.startsWith('image/')) continue
              const file = item.getAsFile()
              if (file) { e.preventDefault(); fileReceiver.current?.(file); break }
            }
          }} />
      </div>
      {store.rights.value.image && <UploadControls visible={!text} receiveFile={fileReceiver} />}
    </div>
    <TypingIndicator />
    {connected && notice && <div class={styles.notice} role="alert">{notice}</div>}
  </div>
}
