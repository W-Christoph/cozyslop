import { signal } from '@preact/signals'
import { useRef, useState } from 'preact/hooks'
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

export function SubtitleCover() {
  const store = useRoomStore()
  const stream = store.stream.value
  const { subtitleCover, subtitleCoverStyle, subtitleCoverBlur, subtitleCoverBox } = preferences.value
  const frame = useRef<HTMLDivElement>(null)
  // The box while it is being dragged; saved when let go.
  const [moving, setMoving] = useState<CoverBox | null>(null)
  if (!subtitleCover || !coverShown.value) return null
  const box = moving ?? fit(subtitleCoverBox)

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
      updatePreferences({ subtitleCoverBox: last })
      setMoving(null)
    }
    target.addEventListener('pointermove', move)
    target.addEventListener('pointerup', end)
    target.addEventListener('pointercancel', end)
  }

  return (
    <div class={styles.area}>
      <div ref={frame} class={styles.frame}>
        <div class={`${styles.cover} ${subtitleCoverStyle === 'black' ? styles.black : ''} ${moving ? styles.moving : ''}`}
          title="Subtitle cover: drag to move, drag an edge to resize"
          style={{ left: `${box.x * 100}%`, top: `${box.y * 100}%`, width: `${box.width * 100}%`, height: `${box.height * 100}%`, '--blur': `${subtitleCoverBlur}px` }}
          onPointerDown={start([])}>
          {/* Blur: a blurred copy of the stream, clipped to the box. Browsers
              do not reliably apply backdrop-filter over a playing video. */}
          {subtitleCoverStyle === 'blur' && <div class={styles.clip}>
            <video class={styles.copy} muted autoplay playsInline aria-hidden="true"
              ref={(el) => { if (el && el.srcObject !== stream) el.srcObject = stream }}
              style={{ left: `${-box.x / box.width * 100}%`, top: `${-box.y / box.height * 100}%`, width: `${100 / box.width}%`, height: `${100 / box.height}%` }} />
          </div>}
          {HANDLES.map((edges) => <span key={edges.join('')} class={styles.handle} data-edge={edges.join('')} onPointerDown={start(edges)} />)}
        </div>
      </div>
    </div>
  )
}
