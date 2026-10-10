// The room's desktop: plays the stream and, while this tab holds the remote,
// forwards mouse and keyboard input to neko.

import { useEffect, useRef } from 'preact/hooks'
import type { RefObject } from 'preact'
import { preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import GuacamoleKeyboard from '../../neko/guacamole-keyboard.js'
import styles from './RemoteScreen.module.css'
import { useTouchTrackpad } from './useTouchTrackpad'
import { useDesktopPaste } from './useDesktopPaste'

interface Props {
  mobile: boolean
  pointer: RefObject<{ x: number; y: number }>
  video: RefObject<HTMLVideoElement | null>
  onPlaybackBlocked: (blocked: boolean) => void
}

const MOUSE_MOVE_THROTTLE_MS = 10
// Browsers report wheel deltas in pixels; one X11 scroll step is roughly this.
const WHEEL_STEP_PX = 53
// A pause this long starts a new scroll gesture.
const WHEEL_GESTURE_MS = 250

export function RemoteScreen({ mobile, pointer, video, onPlaybackBlocked }: Props) {
  const store = useRoomStore()
  const { neko } = store
  const stream = store.video.value === 'connected' ? store.stream.value : null
  const isHost = store.isHost.value
  const paused = store.paused.value
  const { muted, volume } = preferences.value
  const overlay = useRef<HTMLDivElement>(null)
  const clipboard = useRef<HTMLTextAreaElement>(null)
  const { requestPaste, pending, dialog } = useDesktopPaste()
  const resetKeyboard = useRef(() => {})
  const flushModifiers = useRef(() => {})
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
        // A refused or interrupted play() says nothing final: autoplay may
        // already have started the video, or start it later (onPlaying).
        if (!cancelled && el.paused) onPlaybackBlocked(true)
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
    const forwarded = new Set<number>()
    const modifiers = new Set<number>()
    const flush = () => {
      modifiers.forEach((keysym) => { forwarded.add(keysym); neko.keyDown(keysym) })
      modifiers.clear()
    }
    flushModifiers.current = flush
    // Dropping the held-back modifiers first keeps a reset from tapping them.
    const reset = () => { modifiers.clear(); keyboard.reset() }
    const blocked = () => pending.current || !!document.querySelector('[role="dialog"]')
    keyboard.onkeydown = (keysym: number) => {
      if (!hostRef.current || blocked()) return true // let the browser handle it
      // Wait for the next key/mouse action before sending Ctrl/Meta. A local
      // paste must not leave input on the data channel racing control/paste.
      if ([0xffe3, 0xffe4, 0xffe7, 0xffe8, 0xffeb, 0xffec].includes(keysym)) {
        modifiers.add(keysym)
        return false
      }
      flush()
      forwarded.add(keysym)
      neko.keyDown(keysym)
      return false
    }
    keyboard.onkeyup = (keysym: number) => {
      // A modifier released on its own (Super for the menu) is still a key press.
      if (modifiers.delete(keysym) && hostRef.current && !blocked()) { neko.keyDown(keysym); neko.keyUp(keysym) }
      else if (forwarded.delete(keysym) && hostRef.current) neko.keyUp(keysym)
    }
    // Run before Guacamole's capture listener. Firefox targets paste at the
    // body for a non-editable div; a focused textarea works in both browsers.
    // Never cancel the native paste action or forward V (including its keyup).
    const pasteKey = (e: KeyboardEvent) => {
      if (blocked()) { e.stopImmediatePropagation(); return }
      if (!(e.ctrlKey || e.metaKey) || e.key.toLowerCase() !== 'v') return
      e.stopImmediatePropagation()
      reset()
      if (e.type === 'keydown' && hostRef.current) clipboard.current?.focus({ preventScroll: true })
    }
    el.addEventListener('keydown', pasteKey, true)
    el.addEventListener('keyup', pasteKey, true)
    keyboard.listenTo(el)
    resetKeyboard.current = reset
    el.addEventListener('blur', reset)
    const offHost = neko.on('host', reset)
    return () => {
      reset()
      keyboard.onkeydown = null
      keyboard.onkeyup = null
      el.removeEventListener('blur', reset)
      el.removeEventListener('keydown', pasteKey, true)
      el.removeEventListener('keyup', pasteKey, true)
      offHost()
      resetKeyboard.current = () => {}
      flushModifiers.current = () => {}
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
      flushModifiers.current()
      const { x, y } = toScreen(e)
      neko.move(x, y)
      neko.buttonDown(e.button + 1)
      window.addEventListener('mouseup', onUp, true)
    }
    const onUp = (e: MouseEvent) => {
      if (hostRef.current) {
        e.preventDefault()
        const { x, y } = toScreen(e)
        neko.move(x, y)
        neko.buttonUp(e.button + 1)
      }
      if (e.buttons === 0) stopReleaseListener()
    }
    const stopReleaseListener = () => window.removeEventListener('mouseup', onUp, true)
    const onBlur = () => { neko.releaseButtons(); stopReleaseListener() }
    const offHost = neko.on('host', () => { if (!neko.isHost) stopReleaseListener() })
    const offStatus = neko.on('status', (status) => { if (status === 'disconnected') stopReleaseListener() })

    let wheelX = 0
    let wheelY = 0
    let wheelTime = 0
    // At most one scroll step per wheel event, as the old CozyCast did: a
    // notch of a mouse wheel is one step, whatever delta the browser reports
    // for it (100 pixels, or three lines). Small deltas (touchpads) add up to
    // a step; the first event of a gesture scrolls at once.
    const onWheel = (e: WheelEvent) => {
      if (!hostRef.current) return
      e.preventDefault()
      const first = e.timeStamp - wheelTime > WHEEL_GESTURE_MS
      wheelTime = e.timeStamp
      if (first) { wheelX = 0; wheelY = 0 }
      const lineScale = e.deltaMode === WheelEvent.DOM_DELTA_PIXEL ? 1 : WHEEL_STEP_PX
      wheelX += e.deltaX * lineScale
      wheelY += e.deltaY * lineScale
      const step = (delta: number) => (first || Math.abs(delta) >= WHEEL_STEP_PX ? Math.sign(delta) : 0)
      const dx = step(wheelX)
      const dy = step(wheelY)
      if (dx !== 0) wheelX = 0
      if (dy !== 0) wheelY = 0
      if (dx === 0 && dy === 0) return
      // neko maps positive deltas to scroll up/left; browsers use down/right.
      neko.scroll(-dx, -dy, e.ctrlKey)
    }
    const onContextMenu = (e: Event) => {
      if (hostRef.current) e.preventDefault()
    }

    el.addEventListener('mousemove', onMove)
    el.addEventListener('mousedown', onDown)
    window.addEventListener('blur', onBlur)
    el.addEventListener('wheel', onWheel, { passive: false })
    el.addEventListener('contextmenu', onContextMenu)
    return () => {
      el.removeEventListener('mousemove', onMove)
      el.removeEventListener('mousedown', onDown)
      onBlur()
      offHost()
      offStatus()
      window.removeEventListener('blur', onBlur)
      el.removeEventListener('wheel', onWheel)
      el.removeEventListener('contextmenu', onContextMenu)
    }
  }, [neko, mobile, pointer])

  // Copy out: whatever the host copies on the desktop lands on the local
  // clipboard. Browsers only allow this in a focused tab (Firefox also wants
  // a recent key press or click, which the copy itself usually is) and over
  // HTTPS or localhost; otherwise it silently does nothing.
  useEffect(
    () =>
      neko.on('clipboard', (text) => {
        if (!hostRef.current || !text || !document.hasFocus()) return
        void navigator.clipboard?.writeText(text).catch(() => {})
      }),
    [neko],
  )

  useTouchTrackpad(overlay, pointer, mobile)

  return (
    <div class={styles.screen}>
      <video ref={video} class={styles.video} autoplay playsInline onPlaying={() => onPlaybackBlocked(false)} />
      <div ref={overlay} class={styles.overlay} data-host={isHost} tabIndex={0} aria-label="Remote desktop"
        onPaste={(e) => {
          e.preventDefault()
          resetKeyboard.current()
          const text = e.clipboardData?.getData('text/plain')
          if (store.isHost.value && text) requestPaste(text)
          // The dialog restores this focus after either choice.
          overlay.current?.focus({ preventScroll: true })
        }}>
        <textarea ref={clipboard} class={styles.clipboard} tabIndex={-1} aria-label="Desktop clipboard input"
          autocomplete="off" autocapitalize="off" spellcheck={false}
          onInput={(e) => { e.currentTarget.value = '' }} />
      </div>
      {dialog}
    </div>
  )
}
