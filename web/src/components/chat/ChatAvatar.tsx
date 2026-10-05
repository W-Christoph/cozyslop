import { useState } from 'preact/hooks'
import styles from './ChatAvatar.module.css'

// The picture beside a chat message. Without one to show (pictures are loaded
// manually, the file is gone) it is the first letter of the name on its colour.
export function ChatAvatar({ nickname, color, url, class: className }: { nickname: string; color: string; url?: string; class?: string }) {
  const [failed, setFailed] = useState<string | null>(null)
  return (
    <div class={`${styles.avatar} ${className ?? ''}`} style={{ backgroundColor: color }} aria-hidden="true">
      {url && failed !== url
        ? <img src={url} alt="" onError={() => setFailed(url)} />
        : <span>{[...nickname.trim()][0]?.toUpperCase() ?? '?'}</span>}
    </div>
  )
}
