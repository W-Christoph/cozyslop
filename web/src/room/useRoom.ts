import { useEffect, useMemo } from 'preact/hooks'
import { meLoaded, preferences } from '../app/state'
import { RoomStore } from './store'

// useRoom joins a room for as long as the calling component is mounted.
export function useRoom(room: string, access?: string): RoomStore {
  const store = useMemo(
    () => new RoomStore(room, access, { audioOnly: preferences.peek().audioOnly, autoConnect: false }),
    [room, access],
  )
  useEffect(() => () => store.dispose(), [store])
  // Render the room while resolving the session, but join only after any
  // legacy login has finished so the socket uses the right identity.
  const ready = meLoaded.value
  useEffect(() => { if (ready) store.connect() }, [store, ready])
  return store
}
