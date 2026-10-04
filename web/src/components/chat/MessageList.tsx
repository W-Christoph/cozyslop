import { useCallback, useLayoutEffect, useRef, useState } from 'preact/hooks'
import type { ChatMessage } from '../../room/protocol'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { MessageGroup } from './MessageGroup'
import { groupMessages, messageTime } from './parseMessage'
import type { TemporaryLine } from './useChatEvents'
import styles from './MessageList.module.css'

export function MessageList({ lines, editing, onEdit, onEndEdit, onMedia }: {
  lines: readonly TemporaryLine[]
  editing: number | null
  onEdit: (id: number) => void
  onEndEdit: () => void
  onMedia: (id: number) => void
}) {
  const store = useRoomStore()
  const chat = store.chat.value
  const viewport = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const following = useRef(true)
  const userScrollUntil = useRef(0)
  const [history, setHistory] = useState(false)
  const [unread, setUnread] = useState(0)
  const previousChat = useRef(chat)
  const scrollToBottom = useCallback(() => {
    if (following.current && viewport.current) viewport.current.scrollTop = viewport.current.scrollHeight
  }, [])
  const jump = () => {
    following.current = true
    setHistory(false)
    setUnread(0)
    scrollToBottom()
  }
  useLayoutEffect(() => {
    const old = new Set(previousChat.current.map((m) => m.id))
    const added = chat.filter((m) => !old.has(m.id)).length
    if (!following.current && added) setUnread((n) => n + added)
    previousChat.current = chat
    scrollToBottom()
  }, [chat, lines, editing, scrollToBottom])
  useLayoutEffect(() => {
    const observer = new ResizeObserver(scrollToBottom)
    if (content.current) observer.observe(content.current)
    if (viewport.current) observer.observe(viewport.current)
    window.addEventListener('resize', scrollToBottom)
    return () => { observer.disconnect(); window.removeEventListener('resize', scrollToBottom) }
  }, [scrollToBottom])
  const temporary = preferences.value.showLeaveJoinMsg ? lines : []
  const anchored = new Map<number | null, TemporaryLine[]>()
  for (const line of temporary) anchored.set(line.after, [...(anchored.get(line.after) ?? []), line])
  const entries: (ChatMessage[] | TemporaryLine)[] = []
  let run: ChatMessage[] = []
  const flush = () => { entries.push(...groupMessages(run)); run = [] }
  entries.push(...(anchored.get(null) ?? []))
  for (const message of chat) {
    run.push(message)
    const after = anchored.get(message.id)
    if (after?.length) { flush(); entries.push(...after) }
  }
  flush()
  return <div class={styles.list} ref={viewport} tabIndex={0} role="region" aria-label="Chat messages"
    onWheel={() => { userScrollUntil.current = Date.now() + 1000 }}
    onPointerDown={() => { userScrollUntil.current = Date.now() + 60_000 }}
    onPointerUp={() => { userScrollUntil.current = Date.now() + 1000 }}
    onTouchMove={() => { userScrollUntil.current = Date.now() + 1000 }}
    onKeyDown={(e) => { if (['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '].includes(e.key)) userScrollUntil.current = Date.now() + 1000 }}
    onScroll={(e) => {
      const el = e.currentTarget
      const stopped = el.scrollHeight - el.scrollTop - el.clientHeight > 30
      if (!stopped) { following.current = true; setHistory(false); setUnread(0) }
      else if (Date.now() < userScrollUntil.current) { following.current = false; setHistory(true) }
    }}>
    <div ref={content}>
      {entries.map((entry) => Array.isArray(entry)
        ? <MessageGroup key={`message:${entry[0].id}`} messages={entry} editing={editing} onEdit={onEdit} onEndEdit={onEndEdit} onMedia={onMedia} onLoad={scrollToBottom} />
        : <div data-chat-bubble class={styles.temporary} key={`temporary:${entry.id}`} title={messageTime(entry.time)}><div class={styles.hoverTime}>{messageTime(entry.time)}</div><span>{entry.body}</span></div>)}
    </div>
    {history && <button type="button" class={styles.badge} onClick={jump} aria-label={unread ? `${unread} new messages. Jump to bottom` : 'Jump to bottom'}>
      <span class={styles.badgeLabel}>{unread ? `${unread} new messages` : 'Scroll Stopped'}</span><span class={styles.resume}>Jump to bottom</span>
    </button>}
  </div>
}
