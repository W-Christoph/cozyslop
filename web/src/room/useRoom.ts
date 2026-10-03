import { useEffect, useMemo } from 'preact/hooks'
import { preferences } from '../app/state'
import { RoomStore } from './store'

// useRoom joins a room for as long as the calling component is mounted.
export function useRoom(room: string, access?: string): RoomStore {
  const store = useMemo(
    () => new RoomStore(room, access, { audioOnly: preferences.peek().audioOnly }),
    [room, access],
  )
  useEffect(() => () => store.dispose(), [store])
  return store
}
