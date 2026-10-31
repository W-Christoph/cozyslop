import assert from 'node:assert/strict'
import test from 'node:test'
import { createServer } from 'vite'

const server = await createServer({ configFile: false, server: { middlewareMode: true, ws: false } })
let NekoClient
try { ({ NekoClient } = await server.ssrLoadModule('/src/neko/client.ts')) }
finally { await server.close() }

const turn = () => new Promise(resolve => setImmediate(resolve))
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}

function fixture(t) {
  const peers = [], sockets = [], behaviors = [], closed = [], streams = [], timers = new Map()
  class Socket {
    static OPEN = 1
    readyState = Socket.OPEN
    sent = []
    constructor() { sockets.push(this) }
    send(data) { this.sent.push(JSON.parse(data)) }
    close() { this.readyState = 3 }
    message(event, payload) { this.onmessage?.({ data: JSON.stringify({ event, payload }) }) }
  }
  class Peer {
    connectionState = 'new'
    remoteDescription = null
    localDescription = null
    calls = []
    candidates = []
    closed = false
    constructor() {
      this.behavior = behaviors.shift() ?? {}
      if (this.behavior.constructorError) throw this.behavior.constructorError
      this.id = peers.length
      peers.push(this)
    }
    async setRemoteDescription(description) {
      this.calls.push(['remote', description.sdp])
      await this.behavior.remote
      this.remoteDescription = description
    }
    async addIceCandidate(candidate) {
      this.calls.push(['candidate', candidate.candidate])
      await this.behavior.candidate
      this.candidates.push(candidate)
    }
    async createAnswer() {
      this.calls.push(['answer'])
      await this.behavior.answer
      return { type: 'answer', sdp: `answer-${this.id}` }
    }
    async setLocalDescription(description) {
      this.calls.push(['local', description.sdp])
      await this.behavior.local
      this.localDescription = description
    }
    close() { this.closed = true }
  }
  const globals = {
    WebSocket: Socket,
    RTCPeerConnection: Peer,
    location: { protocol: 'https:', host: 'cozy.test' },
    window: {
      setInterval(fn) { const id = timers.size + 1; timers.set(id, fn); return id },
      clearInterval(id) { timers.delete(id) },
    },
  }
  const originals = Object.keys(globals).map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)])
  Object.assign(globalThis, globals)
  const client = new NekoClient()
  client.on('closed', reason => closed.push(reason))
  client.on('stream', stream => streams.push(stream))
  const connect = () => {
    client.connect('/neko/default', 'token')
    const socket = sockets.at(-1)
    socket.onopen()
    return socket
  }
  const socket = connect()
  t.after(() => {
    client.disconnect()
    for (const [key, original] of originals) {
      if (original) Object.defineProperty(globalThis, key, original)
      else delete globalThis[key]
    }
  })
  return {
    client, peers, sockets, socket, behaviors, closed, streams, timers, connect,
    answers: () => sockets.flatMap(s => s.sent).filter(message => message.event === 'signal/answer'),
  }
}

test('a replacement closes the old peer and ignores its queued callbacks', async t => {
  const f = fixture(t)
  await f.client.onMessage('signal/provide', { sdp: 'first' })
  const old = f.peers[0]
  const callbacks = { ice: old.onicecandidate, track: old.ontrack, channel: old.ondatachannel, state: old.onconnectionstatechange }
  await f.client.onMessage('signal/provide', { sdp: 'second' })
  assert.equal(old.closed, true)
  assert.equal(old.ontrack, null)
  const sent = f.socket.sent.length
  callbacks.ice({ candidate: { toJSON: () => ({ candidate: 'old' }) } })
  callbacks.track({ receiver: {}, streams: ['old-stream'], track: { kind: 'video' } })
  callbacks.channel({ channel: { readyState: 'open' } })
  old.connectionState = 'failed'
  callbacks.state()
  assert.equal(f.socket.sent.length, sent)
  assert.deepEqual(f.streams, [])
  assert.equal(f.client.channel, undefined)
  assert.deepEqual(f.closed, [])
  f.peers[1].ontrack({ receiver: {}, streams: ['current-stream'], track: { kind: 'video' } })
  assert.deepEqual(f.streams, ['current-stream'])
  f.client.disconnect()
  assert.ok(f.peers.every(peer => peer.closed))
})

for (const stage of ['remote', 'candidate', 'answer', 'local']) {
  test(`replacement stops stale negotiation after awaiting ${stage}`, async t => {
    const f = fixture(t), gate = deferred()
    f.behaviors.push({ [stage]: gate.promise })
    if (stage === 'candidate') await f.client.onMessage('signal/candidate', { candidate: 'early' })
    const first = f.client.onMessage('signal/provide', { sdp: 'first' })
    await turn()
    assert.ok(f.peers[0].calls.some(([step]) => step === stage), 'the old negotiation reached the delayed step')
    await f.client.onMessage('signal/provide', { sdp: 'second' })
    gate.resolve()
    await first
    assert.deepEqual(f.answers().map(message => message.payload.sdp), ['answer-1'])
    assert.equal(f.peers[0].closed, true)
    assert.equal(f.peers[1].closed, false)
    assert.deepEqual(f.closed, [])
  })
}

test('offers for the same peer negotiate in order', async t => {
  const f = fixture(t), gate = deferred()
  f.behaviors.push({ remote: gate.promise })
  const first = f.client.onMessage('signal/provide', { sdp: 'first' })
  await turn()
  const next = f.client.onMessage('signal/offer', { sdp: 'renegotiate' })
  await turn()
  assert.deepEqual(f.peers[0].calls, [['remote', 'first']])
  gate.resolve()
  await Promise.all([first, next])
  assert.deepEqual(f.peers[0].calls, [
    ['remote', 'first'], ['answer'], ['local', 'answer-0'],
    ['remote', 'renegotiate'], ['answer'], ['local', 'answer-0'],
  ])
  assert.equal(f.answers().length, 2)
})

for (const inline of [false, true]) {
  test(`candidates preceding a replacement offer survive peer cleanup (${inline ? 'inline' : 'JSON'} ufrag)`, async t => {
    const f = fixture(t)
    const early = { candidate: 'initial' }
    await f.client.onMessage('signal/candidate', early)
    await f.client.onMessage('signal/provide', { sdp: 'a=ice-ufrag:first\r\n' })
    assert.deepEqual(f.peers[0].candidates, [early])
    const next = inline ? { candidate: 'candidate:2 1 udp 1 127.0.0.1 52000 typ host ufrag next' }
      : { candidate: 'replacement', usernameFragment: 'next' }
    await f.client.onMessage('signal/candidate', next)
    assert.deepEqual(f.peers[0].candidates, [early], 'a new offer\'s candidate is not added to the previous peer')
    await f.client.onMessage('signal/provide', { sdp: 'a=ice-ufrag:next\r\n' })
    assert.deepEqual(f.peers[1].candidates, [next])
    assert.equal(f.peers[0].closed, true)
  })
}

test('an error from an old session cannot disconnect or signal on the new session', async t => {
  const f = fixture(t), gate = deferred()
  f.behaviors.push({ remote: gate.promise })
  f.socket.message('signal/provide', { sdp: 'old' })
  await turn()
  const nextSocket = f.connect()
  nextSocket.message('signal/provide', { sdp: 'new' })
  await turn()
  f.peers[1].connectionState = 'connected'
  f.peers[1].onconnectionstatechange()
  gate.reject(new Error('old negotiation was closed'))
  await turn()
  assert.deepEqual(f.closed, [])
  assert.equal(f.client.status, 'connected')
  assert.equal(nextSocket.readyState, 1)
  assert.deepEqual(f.answers().map(message => message.payload.sdp), ['answer-1'])
})

for (const stage of ['remote', 'answer', 'local']) {
  test(`current ${stage} failure closes setup and requests normal recovery`, async t => {
    const f = fixture(t), gate = deferred()
    f.behaviors.push({ [stage]: gate.promise })
    f.socket.message('signal/provide', { sdp: 'broken' })
    await turn()
    gate.reject(new Error('negotiation failed'))
    await turn()
    assert.equal(f.client.status, 'disconnected')
    assert.equal(f.socket.readyState, 3)
    assert.equal(f.peers[0].closed, true)
    assert.equal(f.client.peer, undefined)
    assert.equal(f.timers.size, 0)
    assert.deepEqual(f.closed, ['media negotiation failed'])
    assert.deepEqual(f.answers(), [])
  })
}

test('queued callbacks from an old WebSocket leave its replacement alone', t => {
  const f = fixture(t)
  const old = { open: f.socket.onopen, message: f.socket.onmessage, close: f.socket.onclose, timer: [...f.timers.values()][0] }
  const next = f.connect()
  const sent = next.sent.length
  old.open()
  old.message({ data: JSON.stringify({ event: 'system/disconnect', payload: { message: 'old session ended' } }) })
  old.close()
  old.timer()
  assert.equal(next.readyState, 1)
  assert.equal(next.sent.length, sent)
  assert.equal(f.timers.size, 1)
  assert.deepEqual(f.closed, [])
})

test('failed peer construction enters recovery', async t => {
  const f = fixture(t)
  f.behaviors.push({ constructorError: new Error('invalid configuration') })
  f.socket.message('signal/provide', { sdp: 'offer' })
  await turn()
  assert.equal(f.client.status, 'disconnected')
  assert.equal(f.socket.readyState, 3)
  assert.equal(f.closed.length, 1)
})
