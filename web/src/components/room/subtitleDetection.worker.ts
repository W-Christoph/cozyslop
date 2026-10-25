import { detectSubtitle } from './subtitleDetection'

self.onmessage = (event: MessageEvent<{ data: Uint8ClampedArray; width: number; height: number }[]>) => {
  const start = performance.now()
  const boxes = event.data.map(({ data, width, height }) => detectSubtitle(data, width, height))
  self.postMessage({ boxes, milliseconds: performance.now() - start })
}
