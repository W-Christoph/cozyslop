import type { RefObject } from 'preact'
import { useSubtitleDetection } from './useSubtitleDetection'
import { signal } from '@preact/signals'
import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { preferences, updatePreferences, type CoverBox } from '../../app/state'
import { useRoomStore } from './RoomContext'
import styles from './SubtitleCover.module.css'

// A box over part of the picture, blurred or black, for watching without
// reading the subtitles. Opt in under Settings › Room; a toolbar button shows
// and hides it. Shown until switched off, also from one room to the next.
export const coverShown = signal(false)

type Edge = 'n' | 's' | 'e' | 'w'
const HANDLES: Edge[][] = [['n'], ['s'], ['e'], ['w'], ['n', 'w'], ['n', 'e'], ['s', 'w'], ['s', 'e']]
const MIN = 0.03 // smallest side, as a fraction of the picture

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

// Keeps a stored box inside the picture.
function fit({ x, y, width, height }: CoverBox): CoverBox {
  width = clamp(width, MIN, 1)
  height = clamp(height, MIN, 1)
  return { x: clamp(x, 0, 1 - width), y: clamp(y, 0, 1 - height), width, height }
}

// Moves the box (no edges) or the given edges by dx and dy.
function drag(box: CoverBox, edges: Edge[], dx: number, dy: number): CoverBox {
  let { x, y, width, height } = box
  if (edges.length === 0) return { x: clamp(x + dx, 0, 1 - width), y: clamp(y + dy, 0, 1 - height), width, height }
  if (edges.includes('w')) { const right = x + width; x = clamp(x + dx, 0, right - MIN); width = right - x }
  if (edges.includes('e')) width = clamp(width + dx, MIN, 1 - x)
  if (edges.includes('n')) { const bottom = y + height; y = clamp(y + dy, 0, bottom - MIN); height = bottom - y }
  if (edges.includes('s')) height = clamp(height + dy, MIN, 1 - y)
  return { x, y, width, height }
}

export function SubtitleCover({ video }: { video: RefObject<HTMLVideoElement | null> }) {
  const store = useRoomStore()
  const prefs = preferences.value
  const active = prefs.subtitleCover && coverShown.value
  const automatic = prefs.subtitleCoverDetection === 'region' || prefs.subtitleCoverDetection === 'box'
  const boxes = active ? [fit(prefs.subtitleCoverBox), ...(prefs.subtitleCoverTwoBoxes ? [fit(prefs.subtitleCoverSecondBox)] : [])] : []
  const detection = useSubtitleDetection(video, boxes, active && automatic && !store.paused.value && !store.audioOnly.value, store.stream.value, prefs.subtitleCoverDetectionInterval)
  if (!active) return null
  return <>
    {boxes.map((_, index) => <Cover key={index} boxKey={index === 0 ? 'subtitleCoverBox' : 'subtitleCoverSecondBox'}
      automatic={automatic && !detection.failed} detected={detection.boxes[index] ?? null} />)}
  </>
}

function Cover({ boxKey, automatic, detected }: { boxKey: 'subtitleCoverBox' | 'subtitleCoverSecondBox'; automatic: boolean; detected: CoverBox | null }) {
  const { subtitleCoverStyle, subtitleCoverBlur, subtitleCoverFeather } = preferences.value
  const frame = useRef<HTMLDivElement>(null)
  // The box while it is being dragged; saved when let go.
  const [frameSize, setFrameSize] = useState({ width: 0, height: 0 })
  useLayoutEffect(() => {
    const el = frame.current
    if (!el) return
    const update = () => {
      const { width, height } = el.getBoundingClientRect()
      setFrameSize({ width, height })
    }
    const observer = new ResizeObserver(update)
    observer.observe(el)
    update()
    return () => observer.disconnect()
  }, [])
  const [moving, setMoving] = useState<CoverBox | null>(null)
  const box = moving ?? fit(preferences.value[boxKey])

  const full = { x: 0, y: 0, width: 1, height: 1 }
  const region = !automatic || moving || preferences.value.subtitleCoverDetection === 'box' ? full : detected ?? full
  // The draggable box (or detected region) is the fully covered core.
  // Add feathering outside it without changing its saved coordinates.
  const rect = frameSize
  const feather = subtitleCoverStyle === 'blur' ? subtitleCoverFeather : 0
  const padX = rect?.width ? feather / (rect.width * box.width) : 0
  const padY = rect?.height ? feather / (rect.height * box.height) : 0
  const visual = { x: region.x - padX, y: region.y - padY, width: region.width + padX * 2, height: region.height + padY * 2 }
  const visible = !automatic || !!detected || !!moving

  const start = (edges: Edge[]) => (e: PointerEvent) => {
    const rect = frame.current?.getBoundingClientRect()
    if (e.button !== 0 || !rect?.width || !rect.height) return
    e.preventDefault()
    e.stopPropagation()
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    const from = { x: e.clientX, y: e.clientY }
    let last = box
    const move = (ev: PointerEvent) => {
      last = drag(box, edges, (ev.clientX - from.x) / rect.width, (ev.clientY - from.y) / rect.height)
      setMoving(last)
    }
    const end = () => {
      target.removeEventListener('pointermove', move)
      target.removeEventListener('pointerup', end)
      target.removeEventListener('pointercancel', end)
      updatePreferences({ [boxKey]: last })
      setMoving(null)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', end)
    target.addEventListener('pointercancel', end)
  }

  return (
    <div class={styles.area}>
      <div ref={frame} class={styles.frame}>
        <div class={`${styles.cover} ${moving ? styles.moving : ''}`}
          title="Subtitle cover: drag to move, drag an edge to resize"
          style={{ left: `${box.x * 100}%`, top: `${box.y * 100}%`, width: `${box.width * 100}%`, height: `${box.height * 100}%`, '--blur': `${subtitleCoverBlur}px`, '--feather': `${subtitleCoverFeather}px` }}
          onPointerDown={start([])}>
          {/* Filter the displayed picture directly; mask only the visual so
              the fully covered core and drag handles keep their geometry. */}
          {visible && <div class={`${styles.clip} ${subtitleCoverStyle === 'blur' ? styles.blur : ''} ${subtitleCoverStyle === 'blur' && subtitleCoverFeather > 0 ? styles.feather : ''}`}
            style={{ left: `${visual.x * 100}%`, top: `${visual.y * 100}%`, width: `${visual.width * 100}%`, height: `${visual.height * 100}%` }} />}
          {HANDLES.map((edges) => <span key={edges.join('')} class={styles.handle} data-edge={edges.join('')} onPointerDown={start(edges)} />)}
        </div>
      </div>
    </div>
  )
}
