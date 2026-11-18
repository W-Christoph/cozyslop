import { useEffect, useMemo, useState } from 'preact/hooks'
import { me } from '../../app/state'
import { Modal } from '../Modal'
import { WatchlogView } from '../watchlog/WatchlogView'
import { useRoomStore } from './RoomContext'

// The watchlog over the room, without leaving it: the people who are here
// compared, the show that plays first. For accounts, while the watchlog is on.
export function WatchlogWindow({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const user = me.value
  const users = store.users.value
  // Who left the watchlog is not on it, here either.
  const present = useMemo(() => [...users.values()].filter((u) => !u.anonymous && !u.watchlogOff)
    .map((u) => ({ username: u.username, nickname: u.nickname, nameColor: u.nameColor, avatarUrl: u.avatarUrl })), [users])
  // A suggestion that goes was saved or dismissed: ask again, once the
  // server has written it down.
  const open = store.watchProposal.value !== null
  const [version, setVersion] = useState(0)
  useEffect(() => {
    if (open) return
    const timer = window.setTimeout(() => setVersion((v) => v + 1), 1000)
    return () => window.clearTimeout(timer)
  }, [open])
  if (!user) return null
  return <Modal title="Watchlog" size="xl" onClose={onClose}>
    <WatchlogView user={user} present={present} version={version}
      playing={(store.watchProposal.value ?? store.watchPlaying.value)?.animeId || undefined} />
  </Modal>
}
