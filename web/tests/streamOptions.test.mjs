import assert from 'node:assert/strict'
import test from 'node:test'
import { createServer } from 'vite'

// Load the TypeScript module through Vite, like the other tests.
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let mod
try {
  mod = await server.ssrLoadModule('/src/components/room/admin/streamOptions.ts')
} finally { await server.close() }
const { bitrates, parseScreen, parseStream, pickRate, pickStream, ratesFor, resolutions, scaledSize, scales } = mod

const screens = ['1920x1080@60', '1920x1080@30', '1920x1080@25', '1280x720@60', '1280x720@30', '1368x768@25', '800x600@60'].map(parseScreen)
const streams = ['b2500-s100', 'b1000-s100', 'b1000-s50', 'b2500-s50', 'b4000-s100'].map(parseStream)

test('resolutions are distinct, largest first', () => {
  assert.deepEqual(resolutions(screens), ['1920x1080', '1368x768', '1280x720', '800x600'])
})

test('rates follow the resolution', () => {
  assert.deepEqual(ratesFor(screens, '1920x1080'), [60, 30, 25])
  assert.deepEqual(ratesFor(screens, '1368x768'), [25])
})

test('switching resolution keeps a valid frame rate', () => {
  assert.equal(pickRate([60, 30, 25], 25), 25) // still offered
  assert.equal(pickRate([60, 30], 25), 30) // 25 gone: prefer 30
  assert.equal(pickRate([60], 30), 60) // only one left
  assert.equal(pickRate([25], 30), 25) // closest lower
  assert.equal(pickRate([], 30), undefined)
})

test('bitrates and scales are distinct and ordered', () => {
  assert.deepEqual(bitrates(streams), [1000, 2500, 4000])
  assert.deepEqual(scales(streams), [100, 50])
})

test('a missing bitrate/size combination falls back to the closest stream', () => {
  assert.equal(pickStream(streams, 2500, 50).id, 'b2500-s50')
  assert.equal(pickStream(streams, 4000, 50).id, 'b2500-s50') // same size, nearest bitrate
})

test('scaled sizes match the worker rounding to even pixels', () => {
  assert.deepEqual(scaledSize(1280, 720, 100), [1280, 720])
  assert.deepEqual(scaledSize(1280, 720, 67), [858, 482])
  assert.deepEqual(scaledSize(1366, 768, 75), [1024, 576])
})

test('junk is rejected', () => {
  assert.equal(parseScreen('big'), null)
  assert.equal(parseStream('high'), null)
})
