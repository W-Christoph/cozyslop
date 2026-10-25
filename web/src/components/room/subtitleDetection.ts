import type { CoverBox } from '../../app/state'

// An intentionally conservative, language-independent heuristic for bright
// outlined subtitles. This identifies glyph-like shapes, not their meaning.
export function detectSubtitle(data: Uint8ClampedArray, width: number, height: number): CoverBox | null {
  const count = width * height
  const bright = new Uint8Array(count)
  for (let p = 0; p < count; p++) {
    const i = p * 4
    // White or yellow text, including moderately compressed highlights.
    bright[p] = data[i] > 175 && data[i + 1] > 175 && (data[i + 2] > 150 || data[i + 2] < 130) ? 1 : 0
  }
  const queue = new Int32Array(count)
  const glyphs: { x: number; y: number; w: number; h: number }[] = []
  const marks: typeof glyphs = []
  for (let p = 0; p < count; p++) {
    if (!bright[p]) continue
    let head = 0, tail = 1, dark = 0
    queue[0] = p
    bright[p] = 0
    let x0 = width, y0 = height, x1 = 0, y1 = 0
    while (head < tail) {
      const q = queue[head++], x = q % width, y = Math.floor(q / width)
      x0 = Math.min(x0, x); x1 = Math.max(x1, x)
      y0 = Math.min(y0, y); y1 = Math.max(y1, y)
      for (const n of [x > 0 ? q - 1 : -1, x + 1 < width ? q + 1 : -1, y > 0 ? q - width : -1, y + 1 < height ? q + width : -1]) {
        if (n < 0) continue
        if (bright[n]) { bright[n] = 0; queue[tail++] = n }
        const i = n * 4
        if (Math.max(data[i], data[i + 1], data[i + 2]) < 110) dark++
      }
    }
    const w = x1 - x0 + 1, h = y1 - y0 + 1
    if (h >= 5 && h <= Math.min(64, height * 0.8) && w >= 1 && w <= h * 2 && tail >= 8 && dark >= h * 0.6 && tail / (w * h) < 0.95) {
      glyphs.push({ x: x0, y: y0, w, h })
    }
    // Keep small punctuation separately: it must not count as a letter,
    // but an exclamation mark or period next to a detected word needs cover.
    if (tail >= 2 && w <= 8 && h <= 64 && dark >= Math.max(2, h * 0.6)) {
      marks.push({ x: x0, y: y0, w, h })
    }
    // Busy frames aren't useful subtitle candidates. Bound grouping work.
    if (glyphs.length > 300 || marks.length > 600) return null
  }
  const accepted = new Set<typeof glyphs[number]>()
  for (const seed of glyphs) {
    const row = glyphs.filter(g => Math.abs(g.y + g.h - seed.y - seed.h) < seed.h * 0.3 && g.h >= seed.h * 0.6 && g.h <= seed.h * 1.5).sort((a, b) => a.x - b.x)
    let run: typeof glyphs = []
    const save = () => {
      // Two/three-letter replies need tighter spacing and size agreement
      // than a full line, to avoid accepting arbitrary pairs of highlights.
      const short = run.length >= 2 && run.every((g, i) => {
        const previous = run[i - 1]
        return g.h >= seed.h * 0.8 && g.h <= seed.h * 1.25 &&
          (!previous || g.x - previous.x - previous.w <= seed.h * 0.8)
      })
      if (run.length >= 4 || short) run.forEach(g => accepted.add(g))
    }
    for (const g of row) {
      const previous = run[run.length - 1]
      if (previous && g.x - previous.x - previous.w > seed.h * 2) { save(); run = [] }
      run.push(g)
    }
    save()
  }
  if (!accepted.size) return null
  const letters = [...accepted]
  const items = [...letters, ...marks.filter(mark => letters.some(g =>
    mark.h <= g.h && mark.y >= g.y - g.h * 0.25 && mark.y + mark.h <= g.y + g.h * 1.3 &&
    mark.x + mark.w >= g.x - g.h * 0.65 && mark.x <= g.x + g.w + g.h * 0.65))]
  const pad = Math.max(3, Math.max(...items.map(g => g.h)) * 0.25)
  const x = Math.max(0, Math.min(...items.map(g => g.x)) - pad)
  const y = Math.max(0, Math.min(...items.map(g => g.y)) - pad)
  const right = Math.min(width, Math.max(...items.map(g => g.x + g.w)) + pad)
  const bottom = Math.min(height, Math.max(...items.map(g => g.y + g.h)) + pad)
  return { x: x / width, y: y / height, width: (right - x) / width, height: (bottom - y) / height }
}
