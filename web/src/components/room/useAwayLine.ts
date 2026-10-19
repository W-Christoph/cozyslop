import { useEffect } from 'preact/hooks'
import { useRoomStore } from './RoomContext'

// Tells the store when the tab or its window loses the focus and gets it
// back, for the chat view's line where it was left (store.away).
export function useAwayLine() {
  const store = useRoomStore()
  useEffect(() => {
    const update = () => store.setAway(document.hidden || !document.hasFocus())
    window.addEventListener('blur', update)
    window.addEventListener('focus', update)
    document.addEventListener('visibilitychange', update)
    return () => {
      window.removeEventListener('blur', update)
      window.removeEventListener('focus', update)
      document.removeEventListener('visibilitychange', update)
    }
  }, [store])
}
