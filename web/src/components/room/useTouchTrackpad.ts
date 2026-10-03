import type { RefObject } from 'preact'
import { useEffect } from 'preact/hooks'
import { useRoomStore } from './RoomContext'

export function useTouchTrackpad(
  overlay: RefObject<HTMLDivElement | null>,
  pointer: RefObject<{ x: number; y: number }>,
  mobile: boolean,
) {
  const store = useRoomStore()
  const host = store.isHost.value
  useEffect(() => {
    const el = overlay.current
    if (!el || !mobile || !host) return
    let touchId: number | null = null
    let previous = { x: 0, y: 0 }
    const start = (e: TouchEvent) => {
      if (!store.isHost.value || touchId !== null) return
      const touch = e.changedTouches[0]
      if (!touch) return
      e.preventDefault()
      touchId = touch.identifier
      previous = { x: touch.clientX, y: touch.clientY }
    }
    const move = (e: TouchEvent) => {
      if (!store.isHost.value) return
      const touch = Array.from(e.touches).find((t) => t.identifier === touchId)
      if (!touch) return
      e.preventDefault()
      const { width, height } = store.neko.screen
      const position = pointer.current ?? { x: width / 2, y: height / 2 }
      position.x = Math.max(0, Math.min(width - 1, position.x + touch.clientX - previous.x))
      position.y = Math.max(0, Math.min(height - 1, position.y + touch.clientY - previous.y))
      pointer.current = position
      previous = { x: touch.clientX, y: touch.clientY }
      store.neko.move(Math.round(position.x), Math.round(position.y))
    }
    const end = (e: TouchEvent) => {
      if (Array.from(e.changedTouches).some((t) => t.identifier === touchId)) touchId = null
    }
    el.addEventListener('touchstart', start, { passive: false })
    el.addEventListener('touchmove', move, { passive: false })
    el.addEventListener('touchend', end)
    el.addEventListener('touchcancel', end)
    return () => {
      el.removeEventListener('touchstart', start)
      el.removeEventListener('touchmove', move)
      el.removeEventListener('touchend', end)
      el.removeEventListener('touchcancel', end)
    }
  }, [store, host, mobile, overlay, pointer])
}
