import assert from 'node:assert/strict'
import test from 'node:test'
import { createServer } from 'vite'

// Load the TypeScript module through Vite, like the other tests.
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let mod, NekoClient, buffer
try {
  mod = await server.ssrLoadModule('/src/components/room/admin/streamOptions.ts')
  ;({ NekoClient } = await server.ssrLoadModule('/src/neko/client.ts'))
  buffer = await server.ssrLoadModule('/src/neko/streamBuffer.ts')
} finally { await server.close() }
const { bitrates, parseScreen, parseStream, pickRate, pickStream, presets, ratesFor, resolutions, scaledSize, scales } = mod

test('stream buffer updates audio and video live and Auto restores browser defaults', () => {
  const client = new NekoClient()
  const audio = { jitterBufferTarget: null }, video = { jitterBufferTarget: null }
  client.peer = { getReceivers: () => [audio, video] }
  for (const target of [500, 1000, 2000, 4000, null]) {
    client.setStreamBuffer(target)
    assert.equal(audio.jitterBufferTarget, target)
    assert.equal(video.jitterBufferTarget, target)
  }
})

test('new receivers retain the selected buffer after peer replacement, including sound only', async (t) => {
  const previous = globalThis.RTCPeerConnection
  globalThis.RTCPeerConnection = class {
    getReceivers() { return [] }
  }
  t.after(() => {
    if (previous === undefined) delete globalThis.RTCPeerConnection
    else globalThis.RTCPeerConnection = previous
  })
  const client = new NekoClient()
  client.answer = async () => {}
  client.setStreamBuffer(2000)
  for (const audioOnly of [false, true]) {
    client.audioOnly = audioOnly
    await client.createPeer({ sdp: '' })
    for (const kind of ['audio', 'video']) {
      const receiver = { jitterBufferTarget: null }
      client.peer.ontrack({ receiver, streams: [], track: { kind } })
      assert.equal(receiver.jitterBufferTarget, 2000)
    }
  }
})

test('unsupported or rejecting receivers do not prevent playback', () => {
  const unsupported = {}
  buffer.applyStreamBuffer(unsupported, 1000)
  assert.equal('jitterBufferTarget' in unsupported, false)
  const rejecting = { set jitterBufferTarget(_) { throw new Error('unsupported') } }
  assert.doesNotThrow(() => buffer.applyStreamBuffer(rejecting, 1000))
})

test('invalid buffer preferences cannot exceed browser limits', () => {
  for (const invalid of [null, undefined, NaN, Infinity, '1000']) assert.equal(buffer.normalizeStreamBuffer(invalid), null)
  assert.equal(buffer.normalizeStreamBuffer(-1), 0)
  assert.equal(buffer.normalizeStreamBuffer(5000), 4000)
})

const screens = ['1920x1080@60', '1920x1080@30', '1920x1080@25', '1280x720@60', '1280x720@30', '1368x768@25', '800x600@60'].map(parseScreen)
const streams = ['b2500-s100-veryfast', 'b1000-s100-veryfast', 'b1000-s50-veryfast', 'b2500-s50-veryfast', 'b4000-s100-veryfast',
  'b2500-s100-ultrafast', 'b1000-s50-ultrafast'].map(parseStream)

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

test('bitrates, scales and presets are distinct and ordered', () => {
  assert.deepEqual(bitrates(streams), [1000, 2500, 4000])
  assert.deepEqual(scales(streams), [100, 50])
  assert.deepEqual(presets(streams), ['ultrafast', 'veryfast'])
  assert.deepEqual(presets(['b1-s1-custom', 'b1-s1-medium', 'b1-s1-superfast'].map(parseStream)), ['superfast', 'medium', 'custom'])
})

test('stream ids carry the preset; old ids have none', () => {
  assert.deepEqual(parseStream('b2500-s75-superfast'), { id: 'b2500-s75-superfast', kbps: 2500, scale: 75, preset: 'superfast' })
  assert.equal(parseStream('b2500-s75').preset, '')
})

test('a missing bitrate/size combination falls back to the closest stream', () => {
  assert.equal(pickStream(streams, 2500, 50, 'veryfast').id, 'b2500-s50-veryfast')
  assert.equal(pickStream(streams, 4000, 50, 'veryfast').id, 'b2500-s50-veryfast') // same size, nearest bitrate
  assert.equal(pickStream(streams, 2500, 50, 'ultrafast').id, 'b1000-s50-ultrafast') // the preset wins over the bitrate
  assert.equal(pickStream(streams, 2500, 100, 'medium').id, 'b2500-s100-veryfast') // preset not offered
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
