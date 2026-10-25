import assert from 'node:assert/strict'
import test from 'node:test'
import { createServer } from 'vite'
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let detectSubtitle, createSubtitleTracker
try {
  ({ detectSubtitle } = await server.ssrLoadModule('/src/components/room/subtitleDetection.ts'))
  ;({ createSubtitleTracker } = await server.ssrLoadModule('/src/components/room/subtitleTracking.ts'))
}
finally { await server.close() }

const width = 320, height = 100
function fixture(text, color = [255, 255, 255]) {
  const data = new Uint8ClampedArray(width * height * 4)
  // Simple H-shaped glyphs, separated like a subtitle line.
  for (let n = 0; n < text; n++) {
    for (let y = 60; y < 76; y++) for (let x = 70 + n * 12; x < 78 + n * 12; x++) {
      if (x % 12 !== 10 && x % 12 !== 5 && y !== 67 && y !== 68) continue
      const p = (y * width + x) * 4
      data.set([...color, 255], p)
    }
  }
  return data
}
test('empty, uniformly bright and isolated shapes do not activate a cover', () => {
  assert.equal(detectSubtitle(new Uint8ClampedArray(width * height * 4), width, height), null)
  assert.equal(detectSubtitle(new Uint8ClampedArray(width * height * 4).fill(255), width, height), null)
  assert.equal(detectSubtitle(fixture(1), width, height), null)
})
test('white and yellow glyph rows return a padded regional box', () => {
  for (const color of [[255, 255, 255], [255, 240, 50]]) {
    const box = detectSubtitle(fixture(8, color), width, height)
    assert.ok(box)
    assert.ok(box.x < 70 / width && box.y < 60 / height)
    assert.ok((box.x + box.width) * width >= 162)
    assert.ok(box.width < 0.5 && box.height < 0.4)
  }
})
test('unsupported dark lettering is not reported as a confident match', () => {
  assert.equal(detectSubtitle(fixture(8, [50, 50, 50]), width, height), null)
})

test('two and three glyph replies are accepted without requiring a long line', () => {
  for (const length of [2, 3]) {
    assert.ok(detectSubtitle(fixture(length), width, height))
  }
})

test('widely separated pairs of highlights remain rejected', () => {
  const data = fixture(1)
  const other = fixture(1)
  for (let y = 0; y < height; y++) for (let x = 0; x < width - 60; x++) {
    const from = (y * width + x) * 4, to = (y * width + x + 60) * 4
    if (other[from]) data.set(other.subarray(from, from + 4), to)
  }
  assert.equal(detectSubtitle(data, width, height), null)
})

const wide = { x: 0.1, y: 0.2, width: 0.8, height: 0.6 }
const small = { x: 0.3, y: 0.4, width: 0.4, height: 0.2 }
const contains = (outer, inner) => {
  assert.ok(outer.x <= inner.x + 1e-9 && outer.y <= inner.y + 1e-9)
  assert.ok(outer.x + outer.width >= inner.x + inner.width - 1e-9)
  assert.ok(outer.y + outer.height >= inner.y + inner.height - 1e-9)
}
test('bounds expand immediately, hold recent coverage, then ease inward', () => {
  const tracker = createSubtitleTracker()
  tracker.update(small, 0)
  contains(tracker.update(wide, 250), wide)
  const held = tracker.update(small, 500)
  contains(held, wide)
  const shrinking = tracker.update(small, 1000)
  assert.ok(shrinking.width < wide.width && shrinking.width > small.width)
  contains(shrinking, small)
  for (let now = 1250; now <= 4000; now += 250) contains(tracker.update(small, now), small)
  assert.ok(Math.abs(tracker.update(small, 4250).width - small.width) < 0.003)
})
test('brief misses hold coverage, two misses clear it, and boxes track independently', () => {
  const first = createSubtitleTracker(), second = createSubtitleTracker()
  first.update(wide, 0); second.update(small, 0)
  contains(first.update(null, 250), wide)
  assert.equal(first.update(null, 500), null)
  contains(second.update(small, 500), small)
  const restarted = first.update(small, 750)
  assert.ok(Math.abs(restarted.width - small.width) < 1e-9)
})
test('faster sampling preserves coverage through brief detection misses', () => {
  const tracker = createSubtitleTracker()
  tracker.update(wide, 0)
  for (let now = 50; now < 500; now += 50) contains(tracker.update(null, now), wide)
  contains(tracker.update(small, 500), wide)
  for (let now = 550; now < 1000; now += 50) contains(tracker.update(null, now), small)
  assert.equal(tracker.update(null, 1000), null)
})
test('a subtitle moving outside old bounds is covered immediately on every edge', () => {
  const tracker = createSubtitleTracker()
  tracker.update(small, 0)
  const moved = { x: 0, y: 0, width: 1, height: 1 }
  contains(tracker.update(moved, 250), moved)
})

test('punctuation beside a short reply is included without counting as a letter', () => {
  const data = fixture(2)
  for (const [top, bottom] of [[60, 70], [74, 76]]) {
    for (let y = top; y < bottom; y++) for (let x = 94; x < 96; x++) data.set([255, 255, 255, 255], (y * width + x) * 4)
  }
  const box = detectSubtitle(data, width, height)
  assert.ok(box)
  assert.ok((box.x + box.width) * width >= 96)
  assert.ok((box.y + box.height) * height >= 76)
})
