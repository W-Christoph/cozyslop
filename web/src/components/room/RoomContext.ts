import { createContext } from 'preact'
import { signal } from '@preact/signals'
import { useContext } from 'preact/hooks'
import type { RoomStore } from '../../room/store'

// Set by a part of the room that wants the watchlog's window opened; the
// room page opens it and clears this.
export const watchlogWanted = signal(false)

export const RoomContext = createContext<RoomStore | null>(null)

export function useRoomStore(): RoomStore {
  const store = useContext(RoomContext)
  if (!store) throw new Error('Room components must be inside RoomContext')
  return store
}
