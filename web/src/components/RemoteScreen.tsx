// The room's desktop: plays the stream and, while this tab holds the remote,
// forwards mouse and keyboard input to neko.

import { useEffect, useRef } from 'preact/hooks'
import type { NekoClient } from '../neko/client'
import GuacamoleKeyboard from '../neko/guacamole-keyboard.js'
import styles from './RemoteScreen.module.css'

interface Props {
  neko: NekoClient
  stream: MediaStream | null
  isHost: boolean
  muted: boolean
}

const MOUSE_MOVE_THROTTLE_MS = 10
// Browsers report wheel deltas in pixels; one X11 scroll step is roughly this.
const WHEEL_STEP_PX = 53

export function RemoteScreen({ neko, stream, isHost, muted }: Props) {
  const video = useRef<HTMLVideoElement>(null)
  const overlay = useRef<HTMLDivElement>(null)
  const hostRef = useRef(isHost)
  hostRef.current = isHost

  useEffect(() => {
    const el = video.current
    if (!el) return
    el.srcObject = stream
    if (stream) el.play().catch(() => {}) // autoplay may need a user gesture
  }, [stream])

  useEffect(() => {
    if (video.current) video.current.muted = muted
  }, [muted])

  // Keyboard: Guacamole turns browser key events into X11 keysyms. Its
  // listeners live as long as the overlay element.
  useEffect(() => {
    const el = overlay.current
    if (!el) return
    const keyboard = new GuacamoleKeyboard()
    keyboard.onkeydown = (keysym: number) => {
      if (!hostRef.current) return true // let the browser handle it
      neko.keyDown(keysym)
      return false
    }
    keyboard.onkeyup = (keysym: number) => {
      if (hostRef.current) neko.keyUp(keysym)
    }
    keyboard.listenTo(el)
    const reset = () => keyboard.reset()
    el.addEventListener('blur', reset)
    return () => el.removeEventListener('blur', reset)
  }, [neko])

  // Mouse.
  useEffect(() => {
    const el = overlay.current
    if (!el) return

    const toScreen = (e: MouseEvent) => {
      // The video is letterboxed inside the element (object-fit: contain).
      const rect = el.getBoundingClientRect()
      const { width, height } = neko.screen
      const scale = Math.min(rect.width / width, rect.height / height)
      const offsetX = (rect.width - width * scale) / 2
      const offsetY = (rect.height - height * scale) / 2
      const clamp = (v: number, max: number) => Math.max(0, Math.min(max - 1, Math.round(v)))
      return {
        x: clamp((e.clientX - rect.left - offsetX) / scale, width),
        y: clamp((e.clientY - rect.top - offsetY) / scale, height),
      }
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
  }, [neko])

  return (
    <div class={styles.screen}>
      <video ref={video} class={styles.video} autoplay playsInline />
      <div ref={overlay} class={styles.overlay} data-host={isHost} tabIndex={0} />
    </div>
  )
}
