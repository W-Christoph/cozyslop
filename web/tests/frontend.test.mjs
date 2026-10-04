import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

// Exercise component handlers and effect cleanup with browser APIs supplied
// by each fixture, using the same Vite transform as the other Node tests.
const mocks = {
  'preact/hooks': `
    export const useState = (...args) => globalThis.frontendFixture.state(...args)
    export const useRef = (...args) => globalThis.frontendFixture.ref(...args)
    export const useEffect = (...args) => globalThis.frontendFixture.effect(...args)
    export const useLayoutEffect = useEffect
    export const useCallback = fn => fn
    export const useId = () => 'test-dialog'
  `,
  'cropperjs': `export default class {
    constructor() { globalThis.frontendFixture.cropper = this }
    getCroppedCanvas() { return globalThis.frontendFixture.canvas }
    destroy() {}
  }`,
  '/app/state': `export const preferences = { value: { volume: 100, muted: false } }`,
  '/RoomContext': `export const useRoomStore = () => globalThis.frontendFixture.store`,
  '/useTouchTrackpad': `export const useTouchTrackpad = () => {}`,
  '/useChatEvents': `export const useChatEvents = () => []`,
  '/guacamole-keyboard.js': `export default class { listenTo() {} reset() {} }`,
}
const server = await createServer({
  configFile: false,
  server: { middlewareMode: true },
  plugins: [{
    name: 'frontend-test-fixtures',
    enforce: 'pre',
    transform(code, id) {
      if (!id.includes('/src/')) return
      return code.replace(/(from\s*['"])([^'"]+)(['"])/g, (match, before, path, after) => {
        const key = Object.keys(mocks).find((key) => path === key || path.endsWith(key))
        return key ? `${before}virtual:fixture:${key}${after}` : match
      })
    },
    resolveId(id) { if (id.startsWith('virtual:fixture:')) return `\0${id.slice(8)}` },
    load(id) { if (id.startsWith('\0fixture:')) return mocks[id.slice(9)] },
  }],
})
let AvatarChooser, ChatPanel, MessageGroup, MediaModal, RemoteScreen, AccountRow, NekoClient, KickedScreen, DesktopUploadStatus, config
try {
  ;({ AvatarChooser } = await server.ssrLoadModule('/src/components/profile/AvatarChooser.tsx'))
  ;({ ChatPanel } = await server.ssrLoadModule('/src/components/chat/ChatPanel.tsx'))
  ;({ MessageGroup } = await server.ssrLoadModule('/src/components/chat/MessageGroup.tsx'))
  ;({ MediaModal } = await server.ssrLoadModule('/src/components/chat/MediaModal.tsx'))
  ;({ RemoteScreen } = await server.ssrLoadModule('/src/components/room/RemoteScreen.tsx'))
  ;({ AccountRow } = await server.ssrLoadModule('/src/components/admin/AccountRow.tsx'))
  ;({ NekoClient } = await server.ssrLoadModule('/src/neko/client.ts'))
  ;({ KickedScreen } = await server.ssrLoadModule('/src/components/room/KickedScreen.tsx'))
  ;({ DesktopUploadStatus } = await server.ssrLoadModule('/src/components/room/DesktopUpload.tsx'))
  ;({ default: config } = await server.ssrLoadModule('/vite.config.ts'))
} finally { await server.close() }

function fixture(t) {
  const slots = []
  let index = 0, effects = []
  const f = {
    state(value) {
      const i = index++
      slots[i] ??= { value }
      return [slots[i].value, (next) => { slots[i].value = typeof next === 'function' ? next(slots[i].value) : next }]
    },
    ref(value) {
      const i = index++
      slots[i] ??= { current: value }
      return slots[i]
    },
    effect(fn, deps) {
      const i = index++
      if (!slots[i] || deps.some((dep, j) => dep !== slots[i].deps[j])) {
        effects.push(() => {
          slots[i]?.cleanup?.()
          slots[i] = { deps, cleanup: fn() }
        })
      }
    },
    render(Component, props) {
      index = 0
      const node = Component(props)
      // Attach the DOM refs before running effects, as Preact does.
      for (const child of nodes(node)) {
        if (child.ref && f.elements?.[child.type]) child.ref.current = f.elements[child.type]
      }
      const pending = effects
      effects = []
      pending.forEach((fn) => fn())
      return node
    },
    unmount() { slots.forEach((slot) => slot.cleanup?.()) },
  }
  globalThis.frontendFixture = f
  t.after(() => { f.unmount(); delete globalThis.frontendFixture })
  return f
}

function nodes(node) {
  if (!node || typeof node !== 'object') return []
  if (Array.isArray(node)) return node.flatMap(nodes)
  return [node, ...nodes(node.props?.children)]
}
const named = (node, name) => nodes(node).find((n) => n.type?.name === name)
const button = (node, text) => nodes(node).find((n) => n.props?.children === text)

function globals(t, values) {
  const originals = Object.keys(values).map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)])
  Object.assign(globalThis, values)
  t.after(() => {
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor)
      else delete globalThis[key]
    }
  })
}

test('avatar cancellation discards a pending crop before its blob callback', (t) => {
  const f = fixture(t), applied = []
  f.elements = { img: {} }
  const props = { avatar: '', disabled: false, onCrop: (blob) => applied.push(blob) }
  const render = () => f.render(AvatarChooser, props)
  const select = () => nodes(render()).find((n) => n.type === 'input').props.onChange({ currentTarget: { files: [new Blob([], { type: 'image/png' })], value: '' } })
  let finish
  f.canvas = { toBlob(callback) { finish = callback } }
  for (const cancel of ['Close', 'modal', 'replace', 'unmount']) {
    select()
    const node = render()
    button(node, 'Crop').props.onClick()
    if (cancel === 'Close') button(node, 'Close').props.onClick()
    if (cancel === 'modal') named(node, 'Modal').props.onClose()
    if (cancel === 'replace') select()
    if (cancel === 'unmount') f.unmount()
    finish(new Blob(['cancelled']))
    assert.deepEqual(applied, [], cancel)
    render()
  }
})

test('a completed avatar crop is applied once', (t) => {
  const f = fixture(t), applied = []
  f.elements = { img: {} }
  const props = { avatar: '', disabled: false, onCrop: (blob) => applied.push(blob) }
  let node = f.render(AvatarChooser, props), finish
  nodes(node).find((n) => n.type === 'input').props.onChange({ currentTarget: { files: [new Blob([], { type: 'image/png' })], value: '' } })
  f.canvas = { toBlob(callback) { finish = callback } }
  node = f.render(AvatarChooser, props)
  button(node, 'Crop').props.onClick()
  const blob = new Blob(['accepted'])
  finish(blob)
  finish(blob)
  assert.deepEqual(applied, [blob])
  assert.equal(named(f.render(AvatarChooser, props), 'Modal'), undefined)
})

test('image and video previews follow their message and close when it disappears', (t) => {
  const f = fixture(t)
  globals(t, { document: new EventTarget() })
  f.store = { chat: { value: [] }, setTyping() {} }
  const render = () => f.render(ChatPanel, {})
  for (const type of ['image', 'video']) {
    for (const change of ['delete', 'remove media', 'remove message']) {
      const message = { id: 1, type, mediaUrl: '/media/chat/old' }
      f.store.chat.value = [message]
      named(render(), 'MessageList').props.onMedia(1)
      assert.equal(named(render(), 'MediaModal').props.message, message)
      const updated = { ...message, mediaUrl: '/media/chat/new' }
      f.store.chat.value = [updated]
      assert.equal(named(render(), 'MediaModal').props.message, updated)
      f.store.chat.value = change === 'remove message' ? [] : [{ ...updated, ...(change === 'delete' ? { deleted: true } : { mediaUrl: undefined }) }]
      assert.equal(named(render(), 'MediaModal'), undefined)
      f.store.chat.value = [message]
      assert.equal(named(render(), 'MediaModal'), undefined, 'a later welcome must not reopen the preview')
    }
  }
})

test('own verified flag is editable while account lockout controls remain disabled', () => {
  const changes = [], user = { username: 'alice', verified: true, admin: true, disabled: false }
  const props = { user, self: true, busy: false, onUpdate: (...args) => changes.push(args) }
  let node = AccountRow(props)
  assert.equal(button(node, 'verified').props.disabled, false)
  button(node, 'verified').props.onClick()
  assert.deepEqual(changes, [['alice', { verified: false }]])
  for (const text of ['Remove Admin', 'Disable', 'Delete', 'Reset password']) assert.equal(button(node, text).props.disabled, true)
  node = AccountRow({ ...props, busy: true })
  assert.equal(button(node, 'verified').props.disabled, true)
})

test('media clicks select the message ID and the modal reads its current URL', (t) => {
  const f = fixture(t), opened = []
  f.store = { selfKey: { value: 'u:1' }, users: { value: new Map() }, rights: { value: { admin: false } } }
  for (const type of ['image', 'video']) {
    const message = { id: 7, author: 'u:2', type, mediaUrl: '/media/chat/current', time: 0 }
    const node = f.render(MessageGroup, { messages: [message], editing: null, onMedia: (id) => opened.push(id) })
    named(node, 'InlineMedia').props.onOpen({ type, url: '/stale-url' })
    const modal = MediaModal({ message, onClose() {} })
    assert.equal(nodes(modal).find((n) => n.type === (type === 'image' ? 'img' : 'video')).props.src, message.mediaUrl)
  }
  assert.deepEqual(opened, [7, 7])
})

function desktop(t) {
  const f = fixture(t)
  const listeners = new Map()
  const window = new EventTarget()
  const add = window.addEventListener.bind(window), remove = window.removeEventListener.bind(window)
  window.addEventListener = (name, fn) => { listeners.set(name, fn); add(name, fn) }
  window.removeEventListener = (name, fn) => { if (listeners.get(name) === fn) listeners.delete(name); remove(name, fn) }
  window.clearInterval = () => {}
  globals(t, { window })
  const neko = new NekoClient(), sent = []
  neko.sessionId = neko.hostId = 'self'
  neko.channel = { readyState: 'open', send(buffer) { const v = new DataView(buffer); sent.push([v.getUint8(0), v.getUint32(3)]) } }
  f.store = { neko, video: { value: 'disconnected' }, isHost: { value: true }, paused: { value: false } }
  const overlay = new EventTarget()
  overlay.focus = () => {}
  overlay.getBoundingClientRect = () => ({ left: 0, top: 0, width: 1280, height: 720 })
  f.elements = { div: overlay }
  f.render(RemoteScreen, { mobile: false, pointer: { current: null }, video: { current: null }, onPlaybackBlocked() {} })
  const mouse = (target, type, button, buttons) => {
    const e = new Event(type, { cancelable: true })
    Object.assign(e, { button, buttons, clientX: 50, clientY: 50 })
    target.dispatchEvent(e)
  }
  return { f, neko, sent, window, listeners, overlay, down: (button, buttons) => mouse(overlay, 'mousedown', button, buttons), up: (button, buttons) => mouse(window, 'mouseup', button, buttons) }
}

test('desktop releases outside the overlay, including chorded mouse buttons', (t) => {
  const { down, up, sent, listeners } = desktop(t)
  assert.equal(listeners.has('mouseup'), false)
  down(0, 1)
  down(2, 3)
  up(0, 2)
  assert.equal(listeners.has('mouseup'), true)
  up(2, 0)
  assert.deepEqual(sent.filter(([op]) => op === 5 || op === 6), [[5, 1], [5, 3], [6, 1], [6, 3]])
  assert.equal(listeners.has('mouseup'), false)
  up(0, 0)
  assert.equal(sent.filter(([op]) => op === 6).length, 2)
})

test('the desktop file list keeps well-formed entries, and download addresses are escaped', async () => {
  const neko = new NekoClient()
  const lists = []
  neko.on('files', (files) => lists.push(files))
  await neko.onMessage('filetransfer/update', { enabled: true, files: [
    { name: 'movie.mkv', type: 'file', size: 3000000 }, { name: 'folder', type: 'dir' },
    { name: 'no size', type: 'file' }, { name: 5, type: 'file' }, { name: 'link', type: 'symlink' }, null,
  ] })
  await neko.onMessage('filetransfer/update', { enabled: false })
  assert.deepEqual(lists, [[
    { name: 'movie.mkv', type: 'file', size: 3000000 }, { name: 'folder', type: 'dir', size: 0 }, { name: 'no size', type: 'file', size: 0 },
  ], []])
  Object.assign(neko, { path: '/neko/default', token: 'a+b/c' })
  assert.equal(neko.fileUrl('my file&x=1.txt'), '/neko/default/api/filetransfer?token=a%2Bb%2Fc&filename=my%20file%26x%3D1.txt')
})

test('a wheel notch is one scroll step whatever its delta; small deltas add up', (t) => {
  const { overlay, neko } = desktop(t)
  globals(t, { WheelEvent: { DOM_DELTA_PIXEL: 0 } })
  const steps = []
  neko.scroll = (x, y) => steps.push([x + 0, y + 0])
  const wheel = (time, deltaY, deltaMode = 0, deltaX = 0) => {
    const e = new Event('wheel', { cancelable: true })
    Object.defineProperty(e, 'timeStamp', { value: time })
    Object.assign(e, { deltaX, deltaY, deltaMode, ctrlKey: false })
    overlay.dispatchEvent(e)
    assert.equal(e.defaultPrevented, true)
  }
  // A mouse wheel: 100 or more pixels a notch in Chrome, three lines in Firefox.
  wheel(1000, 100); wheel(1050, 125); wheel(1100, 300)
  wheel(2000, 3, 1); wheel(2050, -3, 1)
  assert.deepEqual(steps.splice(0), [[0, -1], [0, -1], [0, -1], [0, -1], [0, 1]])
  // A touchpad: the first event scrolls at once, then one step per 53 pixels.
  wheel(3000, 4); wheel(3016, 20); wheel(3032, 20); wheel(3048, 20); wheel(3064, 20)
  assert.deepEqual(steps.splice(0), [[0, -1], [0, -1]])
  // Sideways, and a new gesture forgets what the last one left over.
  wheel(4000, 0, 0, -30); wheel(4016, 40); wheel(5000, 0, 0, 0)
  assert.deepEqual(steps.splice(0), [[1, 0]])
})

for (const reason of ['blur', 'remote lost', 'disconnect', 'release remote', 'unmount']) {
  test(`held desktop buttons are released on ${reason}`, (t) => {
    const { f, neko, down, up, sent, window } = desktop(t)
    down(0, 1)
    if (reason === 'blur') window.dispatchEvent(new Event('blur'))
    if (reason === 'remote lost') neko.onMessage('control/host', { has_host: true, host_id: 'other' })
    if (reason === 'disconnect') neko.disconnect()
    if (reason === 'release remote') neko.releaseControl()
    if (reason === 'unmount') f.unmount()
    assert.deepEqual(sent.filter(([op]) => op === 6), [[6, 1]])
    up(0, 0)
    assert.deepEqual(sent.filter(([op]) => op === 6), [[6, 1]])
  })
}

test('media uses the same dev proxy target as the API', () => {
  assert.equal(config.server.proxy['/media'].target, config.server.proxy['/api'].target)
  assert.equal(config.server.proxy['/neko'].ws, true)
})

test('unknown rooms show Room not found and a home link', (t) => {
  const f = fixture(t)
  f.store = { kicked: { value: { reason: 'not_found' } } }
  const node = KickedScreen()
  assert.equal(node.props.message, 'Room not found')
  assert.equal(nodes(node).find((n) => n.type === 'a').props.href, '/')
  f.store.kicked.value.reason = 'session'
  assert.equal(KickedScreen().props.message, 'Session expired')
  assert.equal(nodes(KickedScreen()).find((n) => n.props?.href === '/login').props.children, 'Login')
})

test('desktop upload status offers Cancel only while uploading', (t) => {
  const f = fixture(t)
  let cancelled = 0
  f.store = {
    desktopUpload: { value: { state: 'uploading', progress: 0.5, message: 'Uploading…' } },
    cancelDesktopUpload() { cancelled++ },
  }
  const node = DesktopUploadStatus()
  button(node, 'Cancel').props.onClick()
  assert.equal(cancelled, 1)
  assert.equal(nodes(node).find((n) => n.type === 'progress').props.value, 0.5)
  for (const state of ['done', 'error', 'idle']) {
    f.store.desktopUpload.value.state = state
    assert.equal(button(DesktopUploadStatus(), 'Cancel'), undefined)
  }
})

function uploadXHR(t) {
  const instances = []
  class XHR {
    upload = {}
    status = 200
    constructor() { instances.push(this) }
    open(method, url) { Object.assign(this, { method, url }) }
    send(form) { this.form = form }
    abort() { this.aborted = true; if (this.form) this.onabort?.() }
  }
  globals(t, { XMLHttpRequest: XHR })
  const neko = new NekoClient()
  neko.token = 'secret token'
  neko.path = '/neko/default'
  return { neko, instances, files: [new File(['data'], 'movie.mkv')] }
}

test('desktop XHR uploads report progress, allow two hours, and detach abort on completion', async (t) => {
  const { neko, instances, files } = uploadXHR(t)
  const controller = new AbortController(), progress = []
  const upload = neko.upload(files, controller.signal, (p) => progress.push(p))
  const xhr = instances[0]
  assert.equal(xhr.timeout, 2 * 60 * 60 * 1000)
  assert.equal(xhr.method, 'POST')
  assert.equal(xhr.url, '/neko/default/api/filetransfer?token=secret%20token')
  assert.equal(xhr.form.get('files').name, 'movie.mkv')
  xhr.upload.onprogress({ lengthComputable: false })
  xhr.upload.onprogress({ lengthComputable: true, loaded: 1, total: 4 })
  assert.deepEqual(progress, [0.25])
  xhr.onload()
  await upload
  controller.abort()
  assert.equal(xhr.aborted, undefined)
})

for (const reason of ['cancel', 'already cancelled', 'timeout', 'network', 'permission', 'server']) {
  test(`desktop XHR upload reports ${reason} distinctly and removes the abort listener`, async (t) => {
    const { neko, instances, files } = uploadXHR(t)
    const controller = new AbortController()
    if (reason === 'already cancelled') controller.abort()
    const upload = neko.upload(files, controller.signal)
    const rejected = assert.rejects(upload, (error) => {
      if (reason.includes('cancel')) return error.name === 'AbortError' && error.message === 'Upload cancelled.'
      if (reason === 'timeout') return error.name === 'TimeoutError' && error.message === 'Upload timed out.'
      if (reason === 'network') return error.message === 'Upload failed: connection lost.'
      if (reason === 'permission') return error.message === 'You are not allowed to upload files.'
      return error.message === 'Upload failed (500).'
    })
    const xhr = instances[0]
    if (reason === 'cancel') controller.abort()
    if (reason === 'already cancelled') assert.equal(xhr.form, undefined)
    if (reason === 'timeout') xhr.ontimeout()
    if (reason === 'network') xhr.onerror()
    if (reason === 'permission' || reason === 'server') {
      xhr.status = reason === 'permission' ? 403 : 500
      xhr.onload()
    }
    await rejected
    if (reason.includes('cancel')) assert.equal(xhr.aborted, true)
    else {
      controller.abort()
      assert.equal(xhr.aborted, undefined)
    }
  })
}

async function authFixture(t) {
  const requests = [], channels = []
  const document = new EventTarget()
  document.documentElement = { dataset: {} }
  document.visibilityState = 'visible'
  class Channel {
    posted = []
    constructor(name) { this.name = name; channels.push(this) }
    postMessage(value) { this.posted.push(value) }
  }
  globals(t, {
    document,
    localStorage: { getItem() { return null }, setItem() {} },
    BroadcastChannel: Channel,
    fetch(path, init) {
      return new Promise((resolve, reject) => requests.push({ path, init, resolve, reject }))
    },
  })
  const server = await createServer({ configFile: false, server: { middlewareMode: true } })
  let state, api
  try {
    state = await server.ssrLoadModule('/src/app/state.ts')
    ;({ api } = await server.ssrLoadModule('/src/api.ts'))
  } finally { await server.close() }
  const reply = (index, status, data) => requests[index].resolve({ status, ok: status >= 200 && status < 300, json: async () => data })
  return { state, api, requests, channel: channels[0], document, reply }
}

const alice = { username: 'alice', nickname: 'Alice' }
const settle = () => new Promise((resolve) => setImmediate(resolve))

for (const action of ['login', 'register', 'logout']) {
  test(`${action} updates me and broadcasts only after a successful request`, async (t) => {
    const { state, requests, reply, channel } = await authFixture(t)
    state.me.value = action === 'logout' ? alice : null
    const pending = action === 'logout' ? state.logout() : state[action]('alice', 'password', 'invite')
    assert.equal(requests[0].path, `/api/auth/${action}`)
    if (action !== 'logout') {
      assert.deepEqual(JSON.parse(requests[0].init.body), { username: 'alice', password: 'password', ...(action === 'register' ? { inviteCode: 'invite' } : {}) })
    }
    assert.deepEqual(channel.posted, [])
    reply(0, action === 'logout' ? 204 : 200, { user: alice })
    await pending
    assert.deepEqual(state.me.value, action === 'logout' ? null : alice)
    assert.equal(channel.name, 'cozycast-auth')
    assert.deepEqual(channel.posted, ['changed'])
  })
}

test('other-tab auth broadcasts and becoming visible refresh me without polling or rebroadcasting', async (t) => {
  const { state, requests, reply, channel, document } = await authFixture(t)
  state.me.value = alice
  assert.equal(requests.length, 0)
  channel.onmessage({ data: 'changed' })
  assert.equal(requests[0].path, '/api/me')
  reply(0, 200, { user: null })
  await settle()
  assert.equal(state.me.value, null)
  document.visibilityState = 'hidden'
  document.dispatchEvent(new Event('visibilitychange'))
  assert.equal(requests.length, 1)
  document.visibilityState = 'visible'
  document.dispatchEvent(new Event('visibilitychange'))
  assert.equal(requests[1].path, '/api/me')
  reply(1, 200, { user: alice })
  await settle()
  assert.deepEqual(state.me.value, alice)
  assert.deepEqual(channel.posted, [])
})

test('401 from ordinary API requests refreshes me; a login rejection leaves me alone', async (t) => {
  const { state, api, requests, reply, channel } = await authFixture(t)
  state.me.value = alice
  const rejectedLogin = assert.rejects(state.login('wrong', 'wrong'), { status: 401 })
  reply(0, 401, { error: 'Wrong password.' })
  await rejectedLogin
  assert.equal(requests.length, 1)
  assert.equal(state.me.value, alice)
  assert.deepEqual(channel.posted, [])
  const rejectedGet = assert.rejects(api.get('/api/private'), { status: 401 })
  reply(1, 401, { error: 'Session expired.' })
  await rejectedGet
  assert.equal(requests[2].path, '/api/me')
  reply(2, 200, { user: null })
  await settle()
  assert.equal(state.me.value, null)
})

test('401 from /api/me clears me without recursively issuing more requests', async (t) => {
  const { state, requests, reply } = await authFixture(t)
  state.me.value = alice
  const pending = state.refreshMe()
  reply(0, 401, { error: 'Session expired.' })
  await pending
  assert.equal(state.me.value, null)
  assert.equal(state.meLoaded.value, true)
  assert.equal(requests.length, 1)
})

for (const reason of ['network', 'server']) {
  test(`a ${reason} failure reading me keeps the current session`, async (t) => {
    const { state, requests, reply } = await authFixture(t)
    state.me.value = alice
    const pending = state.refreshMe()
    if (reason === 'network') requests[0].reject(new Error('Offline'))
    else reply(0, 503, { error: 'Unavailable.' })
    await pending
    assert.equal(state.me.value, alice)
    assert.equal(state.meLoaded.value, true)
    assert.equal(requests.length, 1)
  })
}

for (const action of ['login', 'register', 'logout']) {
  for (const timing of ['before', 'during']) {
    test(`a me read started ${timing} ${action} cannot overwrite the newer auth result`, async (t) => {
      const { state, reply } = await authFixture(t)
      state.me.value = alice
      let refresh, mutation
      if (timing === 'before') refresh = state.refreshMe()
      mutation = action === 'logout' ? state.logout() : state[action]('alice', 'password')
      if (timing === 'during') refresh = state.refreshMe()
      const authIndex = timing === 'before' ? 1 : 0, meIndex = 1 - authIndex
      reply(authIndex, action === 'logout' ? 204 : 200, { user: alice })
      await mutation
      reply(meIndex, 200, { user: action === 'logout' ? alice : null })
      await refresh
      assert.deepEqual(state.me.value, action === 'logout' ? null : alice)
    })
  }
}

test('a logout broadcast supersedes an older pending me read', async (t) => {
  const { state, channel, requests, reply } = await authFixture(t)
  state.me.value = alice
  const old = state.refreshMe()
  channel.onmessage({ data: 'changed' })
  assert.equal(requests.length, 2)
  reply(1, 200, { user: null })
  await settle()
  assert.equal(state.me.value, null)
  reply(0, 200, { user: alice })
  await old
  assert.equal(state.me.value, null)
})

test('a me read cannot apply after a newer auth action starts', async (t) => {
  const { state, reply } = await authFixture(t)
  state.me.value = alice
  const pending = state.refreshMe()
  const login = state.login('bob', 'password')
  reply(0, 200, { user: null })
  await pending
  assert.equal(state.me.value, alice)
  const bob = { username: 'bob' }
  reply(1, 200, { user: bob })
  await login
  assert.equal(state.me.value, bob)
})

test('an API 401 starts a new me read and supersedes a pending response', async (t) => {
  const { state, api, requests, reply } = await authFixture(t)
  state.me.value = alice
  const old = state.refreshMe()
  const denied = assert.rejects(api.get('/api/private'), { status: 401 })
  reply(1, 401, { error: 'Session expired.' })
  await denied
  assert.equal(requests[2].path, '/api/me')
  reply(2, 200, { user: null })
  await settle()
  reply(0, 200, { user: alice })
  await old
  assert.equal(state.me.value, null)
})

test('becoming visible supersedes an older pending me read', async (t) => {
  const { state, document, requests, reply } = await authFixture(t)
  state.me.value = alice
  const old = state.refreshMe()
  document.dispatchEvent(new Event('visibilitychange'))
  assert.equal(requests.length, 2)
  reply(1, 200, { user: null })
  await settle()
  reply(0, 200, { user: alice })
  await old
  assert.equal(state.me.value, null)
})
