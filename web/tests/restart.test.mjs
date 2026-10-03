import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

// Use the existing Vite transform and fake browser sockets/timers to exercise
// the restart lifecycle without a desktop or additional test dependencies.
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let RoomStore
try {
  ;({ RoomStore } = await server.ssrLoadModule('/src/room/store.ts'))
} finally { await server.close() }

function fixture(t) {
  const timers = new Map()
  let timerID = 0
  class Socket {
    static OPEN = 1
    static instances = []
    readyState = Socket.OPEN
    sent = []
    constructor(url) { this.url = url; Socket.instances.push(this) }
    send(data) { this.sent.push(JSON.parse(data)) }
    close() { this.readyState = 3 }
    message(msg) { this.onmessage?.({ data: JSON.stringify(msg) }) }
  }
  const originals = ['window', 'location', 'WebSocket'].map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)])
  Object.assign(globalThis, {
    window: {
      setTimeout(fn) { timers.set(++timerID, fn); return timerID },
      clearTimeout(id) { timers.delete(id) },
      clearInterval() {},
    },
    location: { protocol: 'http:', host: 'cozy.test' },
    WebSocket: Socket,
  })
  const store = new RoomStore('default')
  const room = Socket.instances[0]
  room.onopen()
  t.after(() => {
    store.dispose()
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor)
      else delete globalThis[key]
    }
  })
  return {
    store, room, timers,
    tick() {
      const pending = [...timers.values()]
      timers.clear()
      pending.forEach((fn) => fn())
    },
    status(status) { store.neko.status = status; store.neko.emit('status', status) },
    welcome(restart) {
      room.message({ type: 'welcome', restart, self: { key: 'u:1' }, rights: { admin: false, trusted: true }, settings: { quality: 'medium' }, users: [], history: [], remote: null })
    },
  }
}

test('availability comes from welcome; restart sends the action and clears old errors', (t) => {
  const { store, room, welcome } = fixture(t)
  assert.equal(store.restartAvailable.value, false)
  welcome(true)
  assert.equal(store.restartAvailable.value, true)
  room.message({ type: 'error', message: 'The room was restarted recently. Try again in 60 minutes.' })
  assert.match(store.error.value, /Try again/)
  store.restart()
  assert.deepEqual(room.sent, [{ type: 'restart' }])
  assert.equal(store.error.value, null)
  welcome(false)
  assert.equal(store.restartAvailable.value, false)
})

test('nickname persists through reconnect and welcome until media connects again', (t) => {
  const { store, room, status, welcome } = fixture(t)
  welcome(true)
  status('connected')
  room.message({ type: 'restarting', by: 'Alice' })
  assert.equal(store.restarting.value, 'Alice')
  status('disconnected')
  welcome(true)
  assert.equal(store.restarting.value, 'Alice')
  room.message({ type: 'neko', token: 'fresh', path: '/neko/default' })
  assert.equal(store.video.value, 'connecting')
  assert.equal(store.restarting.value, 'Alice')
  status('connected')
  assert.equal(store.restarting.value, null)
})

test('failed token requests keep retrying through a restart; success cancels pending retry', (t) => {
  const { store, room, status, tick, timers } = fixture(t)
  room.message({ type: 'restarting', by: 'Alice' })
  status('disconnected')
  store.neko.emit('closed')
  tick()
  assert.deepEqual(room.sent, [{ type: 'neko_token' }])
  for (let attempt = 0; attempt < 2; attempt++) {
    room.message({ type: 'error', message: "The room's desktop is not reachable right now." })
    assert.equal(timers.size, 1)
    assert.equal(store.restarting.value, 'Alice')
    tick()
  }
  assert.equal(room.sent.length, 3)
  room.message({ type: 'error', message: "The room's desktop is not reachable right now." })
  room.message({ type: 'neko', token: 'fresh', path: '/neko/default' })
  assert.equal(timers.size, 0)
  assert.equal(store.error.value, null)
  tick()
  assert.equal(room.sent.length, 3)
  status('connected')
  assert.equal(store.restarting.value, null)
})

test('paused playback does not retry token failures and resumes with a fresh token request', (t) => {
  const { store, room, tick, timers } = fixture(t)
  room.message({ type: 'restarting', by: 'Alice' })
  store.neko.emit('closed')
  store.pause()
  assert.equal(timers.size, 0)
  room.message({ type: 'error', message: "The room's desktop is not reachable right now." })
  tick()
  assert.deepEqual(room.sent, [])
  assert.equal(store.restarting.value, 'Alice')
  store.resume()
  assert.deepEqual(room.sent, [{ type: 'neko_token' }])
})

test('room socket disconnection suppresses pending desktop retry; disposal clears timers', (t) => {
  const { store, room, tick, timers } = fixture(t)
  room.message({ type: 'restarting', by: 'Alice' })
  store.neko.emit('closed')
  room.onclose({ code: 4000 })
  tick()
  assert.deepEqual(room.sent, [])
  room.onopen()
  store.neko.emit('closed')
  assert.equal(timers.size, 1)
  store.dispose()
  assert.equal(timers.size, 0)
})
