import { useState } from 'preact/hooks'
import type { User } from '../../room/protocol'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import styles from './UserAvatar.module.css'

export function UserAvatar({ user, small = false }: { user: User; small?: boolean }) {
  const store = useRoomStore()
  const [loadedUrl, setLoadedUrl] = useState<string | null>(null)
  const [failedUrl, setFailedUrl] = useState<string | null>(null)
  const avatarUrl = user.avatarUrl || (user.anonymous ? '/png/default_avatar_on_alpha.png' : '/png/default_avatar.png')
  const reveal = !preferences.value.manualLoadMedia || loadedUrl === avatarUrl
  const remote = store.remoteHolder.value === user.key
  const picture = reveal && failedUrl !== avatarUrl
  const content = <>
    {picture ? <img class={styles.picture} src={avatarUrl} alt="" onError={() => setFailedUrl(avatarUrl)} />
      : <img class={styles.placeholder} src="/svg/user-silhouette.svg" alt="" />}
  </>
  return (
    <div class={`${styles.container} ${small ? styles.small : ''}`}>
      {!reveal ? <button class={`${styles.avatar} ${user.active ? '' : styles.away}`}
        aria-label={`Load profile picture for ${user.nickname}`} onClick={() => setLoadedUrl(avatarUrl)}>{content}</button>
        : <div class={`${styles.avatar} ${user.active ? '' : styles.away}`} style={user.anonymous ? { backgroundColor: user.nameColor } : undefined}>{content}</div>}
      {remote && <><span class={styles.ring} /><span class={styles.remote} title="Remote holder"><img src="/svg/remoteAlpha.svg" alt="Remote holder" /></span></>}
      {preferences.value.showIfMuted && user.muted && <span class={styles.muted} title="Muted"><img src="/svg/headphone-slash.svg" alt="Muted" /></span>}
    </div>
  )
}
