import type { ChatMessage } from '../../room/protocol'
import { useLayoutEffect, useRef } from 'preact/hooks'
import { preferences } from '../../app/state'
import { useRoomStore } from '../room/RoomContext'
import { ChatAvatar } from './ChatAvatar'
import { InlineMedia } from './InlineMedia'
import { MessageEditor } from './MessageEditor'
import { MessageText } from './MessageText'
import { messageTime } from './parseMessage'
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
  const username = first.anonymous ? `Anon(${first.author.slice(2, 6)})` : author?.username
  const { chatAvatars, chatStyle, manualLoadMedia } = preferences.value
  const avatar = chatAvatars && chatStyle !== 'compact'
  return <div data-chat-bubble class={`${styles.message} ${avatar ? styles.withAvatar : ''}`} ref={bubble}>
    {avatar && <ChatAvatar class={styles.avatarSlot} nickname={first.nickname} color={first.nameColor}
      url={manualLoadMedia || !author ? undefined : author.avatarUrl || (author.anonymous ? '/png/default_avatar_on_alpha.png' : '/png/default_avatar.png')} />}
    <div class={styles.username} style={{ '--name-colour': first.nameColor }} title={username}>
      {first.nickname}
      {username && <div class={styles.realUsername}>{username}</div>}
      <span class={styles.timestamp}>{messageTime(first.time)}</span>
    </div>
    {messages.map((message, index) => message.deleted
      ? <div class={styles.subMessage} key={message.id}>
        {index > 0 && <div class={styles.hoverTime}>{messageTime(message.time)}</div>}
        <div class={styles.deleted}>deleted</div>
      </div>
      : editing === message.id
      ? <MessageEditor key={message.id} message={message} onClose={onEndEdit} />
      : <div class={styles.subMessage} key={message.id} tabIndex={message.id > 0 && (own || store.rights.value.admin) ? 0 : undefined}>
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
