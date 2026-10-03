import { createContext } from 'preact'
import { useContext } from 'preact/hooks'
import type { RoomStore } from '../../room/store'

export const RoomContext = createContext<RoomStore | null>(null)

export function useRoomStore(): RoomStore {
  const store = useContext(RoomContext)
  if (!store) throw new Error('Room components must be inside RoomContext')
  return store
}
