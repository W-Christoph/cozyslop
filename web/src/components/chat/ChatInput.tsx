import type { RefObject } from 'preact'
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { me } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { TypingIndicator } from './TypingIndicator'
import { UploadControls } from './UploadControls'
import { MentionPopup } from './MentionPopup'
import { useMentionSuggestions } from './useMentionSuggestions'
import styles from './ChatInput.module.css'

export function ChatInput({ inputRef, onEdit }: { inputRef: RefObject<HTMLTextAreaElement | null>; onEdit: (id: number) => void }) {
  const store = useRoomStore()
  const [text, setText] = useState('')
  const mentions = useMentionSuggestions(inputRef, text, setText)
  const [notice, setNotice] = useState('')
  const lastTyping = useRef(-Infinity)
  const fileReceiver = useRef<((file: File) => void) | null>(null)
  const connected = store.server.value === 'connected'
  const error = store.error.value
  const anonymous = store.self.value?.anonymous ?? !me.value
  useLayoutEffect(() => {
    const ta = inputRef.current
    const box = ta?.parentElement
    if (!ta || !box) return
    // Measuring collapses the textarea for a moment. Hold its box open
    // meanwhile: if the message list above grew and shrank again, Firefox
    // would leave it scrolled up by the textarea's height.
    box.style.height = getComputedStyle(box).height
    ta.style.height = '0px'
    ta.style.height = `${Math.min(18 * 5, ta.scrollHeight)}px`
    box.style.height = ''
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
    mentions.close()
    setText('')
    stopTyping()
  }
  return <div class={styles.chatbox}>
    <div data-chat-input data-has-text={!!text} class={styles.uploader}>
      {/* The padding around the textarea belongs to the input: a click there writes too. */}
      <div data-chat-input-wrapper class={styles.wrapper}
        onMouseDown={(e) => { if (e.target === e.currentTarget) { e.preventDefault(); inputRef.current?.focus() } }}>
        {/* maxlength as an attribute: Preact clears the maxLength property to 0, which
            blocks typing once a visitor turns out to be logged in. */}
        <textarea {...mentions.inputProps} aria-label="Chat message" placeholder={connected ? undefined : 'Reconnecting…'} ref={inputRef} value={text} rows={1} maxlength={anonymous ? 250 : undefined}
          class={`${styles.textarea} ${!text && store.rights.value.image ? styles.withUploads : ''}`}
          onBlur={() => { stopTyping(); mentions.close() }}
          onInput={(e) => {
            const value = anonymous ? e.currentTarget.value.slice(0, 250) : e.currentTarget.value
            setText(value)
            mentions.syncCaret()
            if (!value) stopTyping()
            else if (Date.now() - lastTyping.current >= 1000) { store.setTyping(true); lastTyping.current = Date.now() }
          }}
          onKeyDown={(e) => {
            if (e.isComposing) return
            if (mentions.onKeyDown(e)) return
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
      <MentionPopup suggestions={mentions} />
      {store.rights.value.image && <UploadControls visible={!text} receiveFile={fileReceiver} />}
    </div>
    <TypingIndicator />
    {connected && notice && <div class={styles.notice} role="alert">{notice}</div>}
  </div>
}
