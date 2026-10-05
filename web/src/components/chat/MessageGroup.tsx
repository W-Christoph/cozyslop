import type { ChatMessage } from '../../room/protocol'
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { userIdentity } from '../room/UserHoverName'
import { ChatAvatar } from './ChatAvatar'
import { InlineMedia } from './InlineMedia'
import { MessageEditor } from './MessageEditor'
import { MessageText } from './MessageText'
import { messageTime } from './parseMessage'
import { applyNameColors } from './nameColor'
import styles from './MessageGroup.module.css'

export function MessageGroup({ messages, editing, onEdit, onEndEdit, onMedia, onLoad }: {
  messages: readonly ChatMessage[]
  editing: number | null
  onEdit: (id: number) => void
  onEndEdit: () => void
  onMedia: (id: number) => void
  onLoad: () => void
}) {
  const store = useRoomStore()
  const bubble = useRef<HTMLDivElement>(null)
  const [actions, setActions] = useState<number | null>(null)
  const activeMessage = useRef<HTMLElement | null>(null)
  useEffect(() => {
    if (actions === null) return
    const outside = (event: PointerEvent) => {
      // The preview a tapped picture opens is not "elsewhere".
      if ((event.target as Element).closest('[role="dialog"]')) return
      if (!activeMessage.current?.contains(event.target as Node)) setActions(null)
    }
    document.addEventListener('pointerdown', outside)
    return () => document.removeEventListener('pointerdown', outside)
  }, [actions])
  useLayoutEffect(() => {
    // A new submessage must reveal the entire bubble again in fullscreen.
    const element = bubble.current
    if (!element) return
    element.style.animationName = 'none'
    void element.offsetWidth
    element.style.removeProperty('animation-name')
  }, [messages.length])
  const first = messages[0]
  const own = first.author === store.selfKey.value
  const author = store.users.value.get(first.author)
  const avatarUrl = author
    ? author.avatarUrl || (author.anonymous ? '/png/default_avatar_on_alpha.png' : '/png/default_avatar.png')
    : first.avatarUrl
  const username = first.anonymous ? userIdentity(first.author) : author && userIdentity(author)
  const { chatAvatars, chatStyle, manualLoadMedia } = preferences.value
  useLayoutEffect(() => applyNameColors(bubble.current))
  const avatar = chatAvatars && chatStyle !== 'compact'
  return <div data-chat-bubble class={`${styles.message} ${avatar ? styles.withAvatar : ''}`} ref={bubble}>
    {avatar && <ChatAvatar class={styles.avatarSlot} nickname={first.nickname} color={first.nameColor}
      url={manualLoadMedia ? undefined : avatarUrl} />}
    <div data-chat-name class={styles.username} style={{ '--name-colour': first.nameColor }} title={username}>
      {first.nickname}
      {username && <div class={styles.realUsername}>{username}</div>}
      <span class={styles.timestamp}>{messageTime(first.time)}</span>
    </div>
    {messages.map((message, index) => message.deleted
      ? <div class={`${styles.subMessage} ${index === messages.length - 1 ? styles.last : ''}`} key={message.id}>
        {index > 0 && <div class={styles.hoverTime}>{messageTime(message.time)}</div>}
        <div class={styles.deleted}>deleted</div>
      </div>
      : editing === message.id
      ? <MessageEditor key={message.id} message={message} onClose={onEndEdit} />
      : <div class={`${styles.subMessage} ${index === messages.length - 1 ? styles.last : ''} ${message.id > 0 && (own || store.rights.value.admin) ? styles.withActions : ''} ${actions === message.id ? styles.actionsShown : ''}`} key={message.id} onPointerUp={(event) => {
        if (event.pointerType !== 'touch' || message.id <= 0 || (!own && !store.rights.value.admin)) return
        // A picture or link still opens; the actions show as well, since such
        // a message may have nothing else to tap.
        if ((event.target as Element).closest(`input, textarea, .${styles.deleteButton}`)) return
        activeMessage.current = event.currentTarget
        setActions(message.id)
      }} tabIndex={message.id > 0 && (own || store.rights.value.admin) ? 0 : undefined}>
        {message.id > 0 && (own || store.rights.value.admin) && <button type="button" class={styles.deleteButton} aria-label="Delete message" onClick={() => store.deleteChat(message.id)}>X</button>}
        {own && message.type === 'text' && message.id > 0 && <button type="button" class={`${styles.deleteButton} ${styles.editButton}`} aria-label="Edit message" onClick={() => onEdit(message.id)}><img src="/svg/edit.svg" alt="" /></button>}
        {index > 0 && <div class={styles.hoverTime}>{messageTime(message.time)}</div>}
        {message.type === 'image' || message.type === 'video'
          ? <InlineMedia media={{ type: message.type, url: message.mediaUrl ?? '' }} onOpen={() => onMedia(message.id)} onLoad={onLoad} />
          : message.type === 'whisper' ? <div class={styles.whisper}>{message.body}</div> : <MessageText body={message.body} />}
        {message.edited && <span class={styles.edited}> (edited)</span>}
      </div>)}
  </div>
}
