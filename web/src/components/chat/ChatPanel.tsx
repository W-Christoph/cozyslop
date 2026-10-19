import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { ChatInput } from './ChatInput'
import { MediaModal } from './MediaModal'
import { MessageList } from './MessageList'
import type { TemporaryLine } from './useChatEvents'
import styles from './ChatPanel.module.css'

// lines: join and leave lines, kept by the room page so that they survive
// the chat moving between the sidebar and the chat view.
// idle: in fullscreen, the pointer has rested and the toolbar is hidden.
// wide: chat has the stream's place (chat view). It then has the look of a
// chat app whatever the chat style: pictures, names and times, no bubbles.
export function ChatPanel({ lines = [], active = true, idle = false, wide = false }: { lines?: readonly TemporaryLine[]; active?: boolean; idle?: boolean; wide?: boolean }) {
  const store = useRoomStore()
  const [editing, setEditing] = useState<number | null>(null)
  const [media, setMedia] = useState<number | null>(null)
  const [fullscreen, setFullscreen] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const chat = store.chat.value
  const mediaMessage = chat.find((m) => m.id === media && !m.deleted && m.mediaUrl && (m.type === 'image' || m.type === 'video'))
  useLayoutEffect(() => {
    const change = () => setFullscreen(!!panel.current && !!document.fullscreenElement?.contains(panel.current))
    change()
    document.addEventListener('fullscreenchange', change)
    return () => document.removeEventListener('fullscreenchange', change)
  }, [])
  useEffect(() => {
    if (editing !== null && !chat.some((m) => m.id === editing && !m.deleted)) setEditing(null)
  }, [chat, editing])
  useEffect(() => {
    if (media !== null && !mediaMessage) setMedia(null)
  }, [media, mediaMessage])
  useEffect(() => {
    if (!active) { store.setTyping(false); setEditing(null); setMedia(null) }
  }, [active, store])
  const endEdit = () => { setEditing(null); input.current?.focus() }
  const { transparentChat, chatScale } = preferences.value
  const chatStyle = wide ? 'modern' : preferences.value.chatStyle
  const transparent = fullscreen && transparentChat
  return <div ref={panel} hidden={!active} class={`${styles.chat} ${transparent ? styles.transparent : ''} ${transparent && idle ? styles.idle : ''}`}
    data-chat-style={chatStyle} data-chat-wide={wide || undefined} data-chat-overlay={transparent || undefined} style={chatScale && chatScale !== 100 ? { fontSize: `${chatScale}%` } : undefined}>
    <MessageList lines={lines} wide={wide} editing={editing} onEdit={setEditing} onEndEdit={endEdit} onMedia={setMedia} />
    <ChatInput inputRef={input} onEdit={setEditing} />
    {mediaMessage && <MediaModal message={mediaMessage} onClose={() => setMedia(null)} />}
  </div>
}
