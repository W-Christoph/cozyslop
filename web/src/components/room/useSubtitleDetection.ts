import type { RefObject } from 'preact'
import { useEffect, useState } from 'preact/hooks'
import type { CoverBox } from '../../app/state'
import { createSubtitleTracker } from './subtitleTracking'

type Detection = { boxes: (CoverBox | null)[]; failed: boolean; milliseconds: number }

// One worker for both boxes, no queued frames, <= 4 samples/second. Pixel
// extraction stays on the UI thread; connected-component analysis does not.
export function useSubtitleDetection(video: RefObject<HTMLVideoElement | null>, boxes: CoverBox[], enabled: boolean, stream: MediaStream | null) {
  const [result, setResult] = useState<Detection>({ boxes: [], failed: false, milliseconds: 0 })
  const coordinates = JSON.stringify(boxes)
  useEffect(() => {
    setResult({ boxes: [], failed: false, milliseconds: 0 })
    if (!enabled || !stream) return
    let worker: Worker | undefined
    let timer = 0, watchdog = 0, stopped = false
    const canvas = document.createElement('canvas')
    const context = canvas.getContext('2d', { willReadFrequently: true })
    const fail = () => {
      stopped = true
      clearTimeout(timer); clearTimeout(watchdog)
      worker?.terminate()
      setResult({ boxes: [], failed: true, milliseconds: 0 })
    }
    let trackers = boxes.map(() => createSubtitleTracker())
    let started = 0
    const sample = () => {
      if (stopped) return
      const el = video.current
      if (document.hidden || !el || el.paused || el.readyState < 2 || !el.videoWidth || !el.getBoundingClientRect().width) {
        trackers = boxes.map(() => createSubtitleTracker())
        setResult({ boxes: [], failed: false, milliseconds: 0 })
        timer = window.setTimeout(sample, 250)
        return
      }
      try {
        if (!context) { fail(); return }
        started = performance.now()
        const frames = boxes.map(box => {
          const w = box.width * el.videoWidth, h = box.height * el.videoHeight
          const scale = Math.min(1, 640 / w, 240 / h)
          canvas.width = Math.max(1, Math.round(w * scale)); canvas.height = Math.max(1, Math.round(h * scale))
          context.drawImage(el, box.x * el.videoWidth, box.y * el.videoHeight, w, h, 0, 0, canvas.width, canvas.height)
          return context.getImageData(0, 0, canvas.width, canvas.height)
        })
        worker!.postMessage(frames.map(({ data, width, height }) => ({ data, width, height })), frames.map(f => f.data.buffer))
        watchdog = window.setTimeout(fail, 3000)
      } catch { fail() }
    }
    try {
      worker = new Worker(new URL('./subtitleDetection.worker.ts', import.meta.url), { type: 'module' })
      worker.onerror = fail
      worker.onmessage = (event: MessageEvent<{ boxes: (CoverBox | null)[]; milliseconds: number }>) => {
        if (stopped) return
        clearTimeout(watchdog)
        const now = performance.now()
        const tracked = event.data.boxes.map((box, i) => trackers[i].update(box, now))
        setResult({ boxes: tracked, failed: false, milliseconds: event.data.milliseconds })
        // Slow devices automatically get fewer samples; never build a backlog.
        const elapsed = performance.now() - started
        timer = window.setTimeout(sample, Math.max(250 - elapsed, elapsed * 4))
      }
      sample()
    } catch { fail() }
    return () => { stopped = true; clearTimeout(timer); clearTimeout(watchdog); worker?.terminate() }
  }, [video, coordinates, enabled, stream])
  return result
}
