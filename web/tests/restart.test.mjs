import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

// Use the existing Vite transform and fake browser sockets/timers to exercise
// the restart lifecycle without a desktop or additional test dependencies.
const server = await createServer({
  configFile: false,
  server: { middlewareMode: true },
  plugins: [{
    name: 'auth-fixture',
    enforce: 'pre',
    transform(code, id) {
      if (id.endsWith('/room/store.ts')) return code.replace("'../app/state'", "'virtual:auth-fixture'")
    },
    resolveId(id) { if (id === 'virtual:auth-fixture') return '\0auth-fixture' },
    load(id) { if (id === '\0auth-fixture') return 'export const refreshMe = async () => { globalThis.authRefreshes++ }' },
  }],
})
let RoomStore
try {
  ;({ RoomStore } = await server.ssrLoadModule('/src/room/store.ts'))
} finally { await server.close() }

function fixture(t, { autoConnect = true } = {}) {
  const timers = new Map()
  const delays = []
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
      setTimeout(fn, delay) { delays.push(delay); timers.set(++timerID, fn); return timerID },
      clearTimeout(id) { timers.delete(id) },
      clearInterval() {},
    },
    location: { protocol: 'http:', host: 'cozy.test' },
    WebSocket: Socket,
  })
  const store = new RoomStore('default', undefined, { autoConnect })
  if (!autoConnect) store.connect()
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
    store, room, timers, delays, sockets: Socket.instances,
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

test('chat retains the latest profile picture through updates, leaving and reconnecting', (t) => {
  const { store, room, welcome } = fixture(t)
  welcome(false)
  const author = { key: 'u:2', username: 'bob', avatarUrl: '/media/avatars/bob.png', anonymous: false }
  room.message({ type: 'user_joined', user: author })
  room.message({ type: 'chat', message: { id: 1, author: author.key, avatarUrl: author.avatarUrl } })
  room.message({ type: 'chat', message: { id: 2, author: 'u:3', avatarUrl: '/media/avatars/other.png' } })
  const other = store.chat.value[1]
  const unchanged = store.chat.value
  room.message({ type: 'user_updated', user: { ...author, muted: true } })
  assert.equal(store.chat.value, unchanged, 'presence changes do not replace chat messages')
  for (const avatarUrl of ['/media/avatars/new.png', '', '/media/avatars/latest.png']) {
    room.message({ type: 'user_updated', user: { ...author, avatarUrl } })
    assert.equal(store.chat.value[0].avatarUrl, avatarUrl || '/png/default_avatar.png')
    assert.equal(store.chat.value[1], other)
  }
  room.message({ type: 'typing', key: author.key, typing: true })
  room.message({ type: 'user_left', key: author.key })
  assert.equal(store.users.value.has(author.key), false)
  assert.equal(store.typing.value.has(author.key), false)
  assert.equal(store.chat.value[0].avatarUrl, '/media/avatars/latest.png')
  const history = store.chat.value
  room.message({ type: 'welcome', self: { key: 'u:1' }, rights: {}, settings: {}, users: [], history, remote: null })
  assert.equal(store.chat.value[0].avatarUrl, '/media/avatars/latest.png')
})

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
    room.message({ type: 'neko_unavailable', message: "The room's desktop is not reachable right now." })
    assert.equal(timers.size, 1)
    assert.equal(store.restarting.value, 'Alice')
    tick()
  }
  assert.equal(room.sent.length, 3)
  room.message({ type: 'neko_unavailable', message: "The room's desktop is not reachable right now." })
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
  room.message({ type: 'neko_unavailable', message: "The room's desktop is not reachable right now." })
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

test('a room joined after session lookup still reconnects automatically after a server restart', (t) => {
  const { store, room, sockets, timers, delays, tick } = fixture(t, { autoConnect: false })
  room.onclose({ code: 1006 })
  assert.equal(store.server.value, 'connecting')
  assert.equal(timers.size, 1)
  assert.equal(delays.at(-1), 1000)
  store.connect()
  assert.equal(sockets.length, 1, 'the existing retry owns reconnecting')
  tick()
  assert.equal(sockets.length, 2)
  assert.equal(sockets[1].url, room.url)
  sockets[1].onopen()
  assert.equal(store.server.value, 'connected')
  sockets[1].onclose({ code: 1006 })
  assert.equal(delays.at(-1), 1000, 'a successful reconnect resets the backoff')
  store.dispose()
  assert.equal(timers.size, 0)
})

test('desktop token failures back off without an announced restart; generic errors do not retry', (t) => {
  const { store, room, tick, timers, welcome, delays } = fixture(t)
  welcome(false)
  room.message({ type: 'error', message: 'Action denied.' })
  assert.equal(timers.size, 0)
  for (let attempt = 0; attempt < 3; attempt++) {
    room.message({ type: 'neko_unavailable', message: 'Desktop unavailable.' })
    assert.equal(timers.size, 1)
    tick()
  }
  assert.deepEqual(room.sent, Array(3).fill({ type: 'neko_token' }))
  assert.deepEqual(delays, [1500, 3000, 6000])
  assert.equal(store.restarting.value, null)
  assert.match(store.error.value, /still trying/)
  room.message({ type: 'neko', token: 'fresh', path: '/neko/default' })
  store.neko.emit('status', 'connected')
  assert.equal(store.error.value, null)
})

for (const reason of ['not_found', 'session', 'kicked', 'banned', 'account', 'verified', 'invite', 'deleted']) {
  test(`${reason} uses the terminal kick path and clears desktop retries`, (t) => {
    const { store, room, timers, tick } = fixture(t)
    globalThis.authRefreshes = 0
    t.after(() => { delete globalThis.authRefreshes })
    room.message({ type: 'neko_unavailable', message: 'Desktop unavailable.' })
    assert.equal(timers.size, 1)
    room.message({ type: 'kicked', reason })
    assert.equal(store.kicked.value.reason, reason)
    assert.equal(timers.size, 0)
    assert.equal(globalThis.authRefreshes, reason === 'session' ? 1 : 0)
    room.message({ type: 'neko_unavailable', message: 'Desktop unavailable.' })
    room.onclose({ code: 4000 })
    tick()
    assert.deepEqual(room.sent, [])
    assert.equal(timers.size, 0)
  })
}

test('room close immediately clears a pending desktop retry', (t) => {
  const { room, timers } = fixture(t)
  room.message({ type: 'neko_unavailable', message: 'Desktop unavailable.' })
  assert.equal(timers.size, 1)
  room.onclose({ code: 4000 })
  assert.equal(timers.size, 0)
  room.message({ type: 'neko_unavailable', message: 'Desktop unavailable.' })
  assert.equal(timers.size, 0)
})

function uploadFixture(t) {
  const f = fixture(t), uploads = []
  f.store.rights.value = { upload: true }
  f.store.neko.upload = (files, signal, progress) => new Promise((resolve, reject) => {
    uploads.push({ files, signal, progress, resolve, reject })
  })
  return { ...f, uploads }
}

test('desktop cancellation ignores stale progress and completion after a replacement upload', async (t) => {
  const { store, uploads, timers } = uploadFixture(t)
  const first = store.uploadToDesktop([{ name: 'first' }])
  uploads[0].progress(0.5)
  assert.equal(store.desktopUpload.value.progress, 0.5)
  store.cancelDesktopUpload()
  assert.equal(uploads[0].signal.aborted, true)
  assert.equal(store.desktopUpload.value.message, 'Upload cancelled.')
  const second = store.uploadToDesktop([{ name: 'second' }])
  assert.equal(timers.size, 0)
  uploads[0].progress(0.9)
  uploads[0].resolve()
  await first
  assert.equal(store.desktopUpload.value.message, 'Uploading second…')
  assert.equal(timers.size, 0)
  uploads[1].resolve()
  await second
  assert.equal(store.desktopUpload.value.message, 'Uploaded second to Downloads.')
  assert.equal(timers.size, 1)
})

for (const reason of ['dispose', 'kick', 'rights', 'welcome']) {
  test(`desktop uploads abort on ${reason} and ignore late failure`, async (t) => {
    const { store, room, uploads, welcome, timers } = uploadFixture(t)
    const pending = store.uploadToDesktop([{ name: 'file' }])
    if (reason === 'dispose') store.dispose()
    if (reason === 'kick') room.message({ type: 'kicked', reason: 'kicked' })
    if (reason === 'rights') room.message({ type: 'rights', rights: { upload: false } })
    if (reason === 'welcome') welcome(false)
    assert.equal(uploads[0].signal.aborted, true)
    const before = store.desktopUpload.value
    uploads[0].progress(1)
    uploads[0].reject(new Error('Late network failure.'))
    await pending
    assert.equal(store.desktopUpload.value, before)
    if (reason === 'dispose') assert.equal(timers.size, 0)
  })
}

test('desktop upload failures report their error and reset through the existing status timer', async (t) => {
  const { store, uploads, tick } = uploadFixture(t)
  const pending = store.uploadToDesktop([{ name: 'file' }])
  uploads[0].reject(new DOMException('Upload timed out.', 'TimeoutError'))
  await pending
  assert.equal(store.desktopUpload.value.message, 'Upload timed out.')
  tick()
  assert.equal(store.desktopUpload.value.state, 'idle')
})

test('generic errors do not initiate desktop recovery during an announced restart', (t) => {
  const { store, room, timers, tick } = fixture(t)
  room.message({ type: 'restarting', by: 'Alice' })
  room.message({ type: 'error', message: 'Action denied.' })
  assert.equal(store.error.value, 'Action denied.')
  assert.equal(timers.size, 0)
  tick()
  assert.deepEqual(room.sent, [])
})

test('desktop file actions resolve with their own answer and fail when the connection drops', async (t) => {
  const { store, room } = fixture(t)
  let listed = 0
  store.neko.requestFiles = () => { listed++ }
  store.desktopFiles.value = [{ name: 'a.mp4', type: 'file', size: 1 }, { name: 'b.mp4', type: 'file', size: 2 }]
  const gone = store.desktopFileAction('delete', 'a.mp4')
  const played = store.desktopFileAction('play', 'b.mp4')
  assert.deepEqual(room.sent.slice(-2), [{ type: 'file_delete', name: 'a.mp4' }, { type: 'file_play', name: 'b.mp4' }])
  room.message({ type: 'file_result', action: 'play', name: 'b.mp4', error: 'Bob owns the remote.' })
  assert.equal(await played, 'Bob owns the remote.')
  assert.equal(store.desktopFiles.value.length, 2)
  room.message({ type: 'file_result', action: 'delete', name: 'a.mp4', error: '' })
  assert.equal(await gone, '')
  assert.deepEqual(store.desktopFiles.value.map((f) => f.name), ['b.mp4'])
  assert.equal(listed, 1)
  // An answer nobody waits for changes nothing.
  room.message({ type: 'file_result', action: 'delete', name: 'b.mp4', error: '' })
  assert.equal(store.desktopFiles.value.length, 1)

  const lost = store.desktopFileAction('delete', 'b.mp4')
  room.onclose({ code: 1006 })
  assert.equal(await lost, 'Connection lost.')
  assert.equal(await store.desktopFileAction('play', 'b.mp4'), 'Not connected to the room.')
})
