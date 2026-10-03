// The room's desktop: plays the stream and, while this tab holds the remote,
// forwards mouse and keyboard input to neko.

import { useEffect, useRef } from 'preact/hooks'
import type { RefObject } from 'preact'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import GuacamoleKeyboard from '../../neko/guacamole-keyboard.js'
import styles from './RemoteScreen.module.css'
import { useTouchTrackpad } from './useTouchTrackpad'

interface Props {
  mobile: boolean
  pointer: RefObject<{ x: number; y: number }>
  video: RefObject<HTMLVideoElement | null>
  onPlaybackBlocked: (blocked: boolean) => void
}

const KEYSYM_V = 0x76
const KEYSYM_SHIFT_V = 0x56
const MOUSE_MOVE_THROTTLE_MS = 10
// Browsers report wheel deltas in pixels; one X11 scroll step is roughly this.
const WHEEL_STEP_PX = 53

export function RemoteScreen({ mobile, pointer, video, onPlaybackBlocked }: Props) {
  const store = useRoomStore()
  const { neko } = store
  const stream = store.video.value === 'connected' ? store.stream.value : null
  const isHost = store.isHost.value
  const paused = store.paused.value
  const { muted, volume } = preferences.value
  const overlay = useRef<HTMLDivElement>(null)
  const hostRef = useRef(isHost)
  hostRef.current = isHost

  useEffect(() => {
    if (!video.current) return
    video.current.muted = muted
    video.current.volume = Math.max(0, Math.min(100, volume)) / 100
  }, [muted, volume])

  useEffect(() => {
    const el = video.current
    if (!el) return
    let cancelled = false
    el.srcObject = stream
    onPlaybackBlocked(false)
    if (paused) el.pause()
    else if (stream) {
      void el.play().catch(() => {
        if (!cancelled) onPlaybackBlocked(true)
      })
    }
    return () => { cancelled = true }
  }, [stream, paused, video, onPlaybackBlocked])

  useEffect(() => {
    pointer.current = { x: neko.screen.width / 2, y: neko.screen.height / 2 }
  }, [stream, neko, pointer])

  // Keyboard: Guacamole turns browser key events into X11 keysyms. Its
  // listeners live as long as the overlay element.
  useEffect(() => {
    const el = overlay.current
    if (!el) return
    const keyboard = new GuacamoleKeyboard()
    keyboard.onkeydown = (keysym: number) => {
      if (!hostRef.current) return true // let the browser handle it
      // Ctrl/Cmd+V: let the browser paste, so the overlay's paste handler
      // can send the *local* clipboard. Sending the keystroke would paste the
      // desktop's own clipboard instead.
      if ((keysym === KEYSYM_V || keysym === KEYSYM_SHIFT_V) && (keyboard.modifiers.ctrl || keyboard.modifiers.meta)) {
        return true
      }
      neko.keyDown(keysym)
      return false
    }
    keyboard.onkeyup = (keysym: number) => {
      if (hostRef.current) neko.keyUp(keysym)
    }
    keyboard.listenTo(el)
    const reset = () => keyboard.reset()
    el.addEventListener('blur', reset)
    return () => {
      keyboard.reset()
      keyboard.onkeydown = null
      keyboard.onkeyup = null
      el.removeEventListener('blur', reset)
    }
  }, [neko])

  // Mouse.
  useEffect(() => {
    const el = overlay.current
    if (!el || mobile) return

    const toScreen = (e: MouseEvent) => {
      // The video is letterboxed inside the element (object-fit: contain).
      const rect = el.getBoundingClientRect()
      const { width, height } = neko.screen
      const scale = Math.min(rect.width / width, rect.height / height)
      const offsetX = (rect.width - width * scale) / 2
      const offsetY = (rect.height - height * scale) / 2
      const clamp = (v: number, max: number) => Math.max(0, Math.min(max - 1, Math.round(v)))
      pointer.current = {
        x: clamp((e.clientX - rect.left - offsetX) / scale, width),
        y: clamp((e.clientY - rect.top - offsetY) / scale, height),
      }
      return pointer.current
    }

    let lastMove = 0
    const onMove = (e: MouseEvent) => {
      if (!hostRef.current || e.timeStamp - lastMove < MOUSE_MOVE_THROTTLE_MS) return
      lastMove = e.timeStamp
      const { x, y } = toScreen(e)
      neko.move(x, y)
    }
    const onDown = (e: MouseEvent) => {
      el.focus()
      if (!hostRef.current) return
      e.preventDefault()
      const { x, y } = toScreen(e)
      neko.move(x, y)
      neko.buttonDown(e.button + 1)
    }
    const onUp = (e: MouseEvent) => {
      if (!hostRef.current) return
      e.preventDefault()
      const { x, y } = toScreen(e)
      neko.move(x, y)
      neko.buttonUp(e.button + 1)
    }

    let wheelX = 0
    let wheelY = 0
    const onWheel = (e: WheelEvent) => {
      if (!hostRef.current) return
      e.preventDefault()
      const lineScale = e.deltaMode === WheelEvent.DOM_DELTA_PIXEL ? 1 : WHEEL_STEP_PX
      wheelX += e.deltaX * lineScale
      wheelY += e.deltaY * lineScale
      const dx = Math.trunc(wheelX / WHEEL_STEP_PX)
      const dy = Math.trunc(wheelY / WHEEL_STEP_PX)
      if (dx === 0 && dy === 0) return
      wheelX -= dx * WHEEL_STEP_PX
      wheelY -= dy * WHEEL_STEP_PX
      // neko maps positive deltas to scroll up/left; browsers use down/right.
      neko.scroll(-dx, -dy, e.ctrlKey)
    }
    const onContextMenu = (e: Event) => {
      if (hostRef.current) e.preventDefault()
    }

    el.addEventListener('mousemove', onMove)
    el.addEventListener('mousedown', onDown)
    el.addEventListener('mouseup', onUp)
    el.addEventListener('wheel', onWheel, { passive: false })
    el.addEventListener('contextmenu', onContextMenu)
    return () => {
      el.removeEventListener('mousemove', onMove)
      el.removeEventListener('mousedown', onDown)
      el.removeEventListener('mouseup', onUp)
      el.removeEventListener('wheel', onWheel)
      el.removeEventListener('contextmenu', onContextMenu)
    }
  }, [neko, mobile, pointer])

  useTouchTrackpad(overlay, pointer, mobile)

  return (
    <div class={styles.screen}>
      <video ref={video} class={styles.video} autoplay playsInline />
      <div ref={overlay} class={styles.overlay} data-host={isHost} tabIndex={0} aria-label="Remote desktop"
        onPaste={(e) => {
          // neko puts the text on the desktop clipboard and presses Ctrl+V there.
          const text = e.clipboardData?.getData('text/plain')
          if (!store.isHost.value || !text) return
          e.preventDefault()
          store.neko.paste(text)
        }} />
    </div>
  )
}
