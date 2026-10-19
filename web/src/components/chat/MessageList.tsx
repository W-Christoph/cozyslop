import { useCallback, useLayoutEffect, useRef, useState } from 'preact/hooks'
import type { ChatMessage } from '../../room/protocol'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { MessageGroup } from './MessageGroup'
import { groupMessages, messageTime } from './parseMessage'
import type { TemporaryLine } from './useChatEvents'
import styles from './MessageList.module.css'

// The line where this visit started (chat view only).
interface JoinedLine { joined: number } // time

// wide: the chat view's look (see ChatPanel), with a line where this visit
// started.
export function MessageList({ lines, wide = false, editing, onEdit, onEndEdit, onMedia }: {
  lines: readonly TemporaryLine[]
  wide?: boolean
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
  const { showLeaveJoinMsg } = preferences.value
  const chatStyle = wide ? 'modern' : preferences.value.chatStyle
  const temporary = showLeaveJoinMsg ? lines : []
  const anchored = new Map<number | null, TemporaryLine[]>()
  for (const line of temporary) anchored.set(line.after, [...(anchored.get(line.after) ?? []), line])
  const entries: (ChatMessage[] | TemporaryLine | JoinedLine)[] = []
  const joined = wide ? store.joined.value : null
  let run: ChatMessage[] = []
  const flush = () => {
    entries.push(...(chatStyle === 'compact' ? run.map((message) => [message]) : groupMessages(run)))
    run = []
  }
  entries.push(...(anchored.get(null) ?? []))
  if (joined && joined.after === null) entries.push({ joined: joined.time })
  for (const message of chat) {
    run.push(message)
    const after = anchored.get(message.id)
    if (after?.length) { flush(); entries.push(...after) }
    if (joined?.after === message.id) { flush(); entries.push({ joined: joined.time }) }
  }
  flush()
  return <div class={styles.list} ref={viewport} tabIndex={0} role="region" aria-label="Chat messages"
    onWheel={() => { userScrollUntil.current = Date.now() + 1000 }}
    onPointerDown={() => { userScrollUntil.current = Date.now() + 60_000 }}
    onPointerUp={() => { userScrollUntil.current = Date.now() + 1000 }}
    onPointerCancel={() => { userScrollUntil.current = Date.now() + 1000 }}
    onTouchMove={() => { userScrollUntil.current = Date.now() + 1000 }}
    onKeyDown={(e) => { if (['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown', 'Home', 'End', ' '].includes(e.key)) userScrollUntil.current = Date.now() + 1000 }}
    onScroll={(e) => {
      const el = e.currentTarget
      const stopped = el.scrollHeight - el.scrollTop - el.clientHeight > 30
      if (!stopped) { following.current = true; setHistory(false); setUnread(0) }
      else if (Date.now() < userScrollUntil.current) { following.current = false; setHistory(true) }
    }}>
    <div ref={content} class={styles.content}>
      {entries.map((entry) => Array.isArray(entry)
        ? <MessageGroup key={`message:${entry[0].id}`} messages={entry} wide={wide} editing={editing} onEdit={onEdit} onEndEdit={onEndEdit} onMedia={onMedia} onLoad={scrollToBottom} />
        : 'joined' in entry ? <div class={styles.joined} key="joined" role="separator"><span>You joined · {messageTime(entry.joined)}</span></div>
        : <div data-chat-bubble class={styles.temporary} key={`temporary:${entry.id}`} title={messageTime(entry.time)}><div class={styles.hoverTime}>{messageTime(entry.time)}</div><span>{entry.body}</span></div>)}
    </div>
    {history && <button type="button" class={styles.badge} onClick={jump} aria-label={unread ? `${unread} new messages. Jump to bottom` : 'Jump to bottom'}>
      <span class={styles.badgeLabel}>{unread ? `${unread} new messages` : 'Scroll Stopped'}</span><span class={styles.resume}>Jump to bottom</span>
    </button>}
  </div>
}
