import { useRoomStore } from '../room/RoomContext'
import { matchedPingNames, parseMessage, pingName } from './parseMessage'
import styles from './MessageText.module.css'

export function MessageText({ body }: { body: string }) {
  const store = useRoomStore()
  const ownName = pingName(store.self.value?.nickname ?? '')
  const names = matchedPingNames(store.users.value)
  return <span class={styles.text}>{parseMessage(body).map((part, i) => {
    if (part.type === 'url') return <a key={i} class={styles.link} href={part.href} target="_blank" rel="noopener noreferrer">{part.text}</a>
    const className = part.type === 'ping'
      ? part.target === ownName ? styles.ping : names.has(part.target) ? styles.otherPing : ''
      : ''
    return <span key={i} class={className}>{part.text.split('\n').map((line, j) => <span key={j}>{j > 0 && <br />}{line}</span>)}</span>
  })}</span>
}
