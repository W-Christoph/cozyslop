import { useRoomStore } from '../room/RoomContext'
import styles from './TypingIndicator.module.css'

export function TypingIndicator() {
  const store = useRoomStore()
  const names = [...store.typing.value].filter((key) => key !== store.selfKey.value)
    .map((key) => store.users.value.get(key)?.nickname).filter((name): name is string => !!name)
  const label = names.length > 2 ? 'Several people are' : names.length === 2 ? `${names[0]} and ${names[1]} are` : `${names[0]} is`
  return <div class={styles.typing} role="status">{names.length > 0 && <>{label} typing<span class={styles.dots} aria-hidden="true"><span /></span><span class={styles.accessible}>…</span></>}</div>
}
