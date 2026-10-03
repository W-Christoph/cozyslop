import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { ChatInput } from './ChatInput'
import type { Media } from './InlineMedia'
import { MediaModal } from './MediaModal'
import { MessageList } from './MessageList'
import { useChatEvents } from './useChatEvents'
import styles from './ChatPanel.module.css'

export function ChatPanel({ active = true }: { active?: boolean }) {
  const store = useRoomStore()
  const lines = useChatEvents(store)
  const [editing, setEditing] = useState<number | null>(null)
  const [media, setMedia] = useState<Media | null>(null)
  const [fullscreen, setFullscreen] = useState(false)
  const input = useRef<HTMLTextAreaElement>(null)
  const panel = useRef<HTMLDivElement>(null)
  const chat = store.chat.value
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
    if (!active) { store.setTyping(false); setEditing(null); setMedia(null) }
  }, [active, store])
  const endEdit = () => { setEditing(null); input.current?.focus() }
  const transparent = fullscreen && preferences.value.transparentChat
  return <div ref={panel} hidden={!active} class={`${styles.chat} ${transparent ? styles.transparent : ''}`}>
    <MessageList lines={lines} editing={editing} onEdit={setEditing} onEndEdit={endEdit} onMedia={setMedia} />
    <ChatInput inputRef={input} onEdit={setEditing} />
    {media && <MediaModal media={media} onClose={() => setMedia(null)} />}
  </div>
}
