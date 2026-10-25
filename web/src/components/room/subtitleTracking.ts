import type { CoverBox } from '../../app/state'

// Keep recent bounds for 200 ms, then ease inward. Outward edges always
// update immediately so smoothing never exposes a newly detected glyph.
export function createSubtitleTracker() {
  let recent: { box: CoverBox; time: number }[] = []
  let shown: CoverBox | null = null
  let misses = 0, lastTime = 0, lastDetection = 0
  return {
    update(box: CoverBox | null, now: number): CoverBox | null {
      const elapsed = Math.max(0, now - lastTime)
      lastTime = now
      if (!box) {
        if (++misses >= 2 && now - lastDetection >= 200) { recent = []; shown = null }
        return shown
      }
      misses = 0
      lastDetection = now
      recent = recent.filter(sample => now - sample.time < 200)
      recent.push({ box, time: now })
      const left = Math.min(...recent.map(s => s.box.x))
      const top = Math.min(...recent.map(s => s.box.y))
      const right = Math.max(...recent.map(s => s.box.x + s.box.width))
      const bottom = Math.max(...recent.map(s => s.box.y + s.box.height))
      const amount = 1 - Math.exp(-elapsed / 100)
      const inward = (from: number, to: number) => Math.abs(to - from) < 0.001 ? to : from + (to - from) * amount
      const x = shown ? Math.min(left, inward(shown.x, left)) : left
      const y = shown ? Math.min(top, inward(shown.y, top)) : top
      const r = shown ? Math.max(right, inward(shown.x + shown.width, right)) : right
      const b = shown ? Math.max(bottom, inward(shown.y + shown.height, bottom)) : bottom
      shown = { x, y, width: r - x, height: b - y }
      return shown
    },
  }
}
