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
let AvatarChooser, ChatPanel, MessageGroup, MediaModal, RemoteScreen, AccountRow, NekoClient, config
try {
  ;({ AvatarChooser } = await server.ssrLoadModule('/src/components/profile/AvatarChooser.tsx'))
  ;({ ChatPanel } = await server.ssrLoadModule('/src/components/chat/ChatPanel.tsx'))
  ;({ MessageGroup } = await server.ssrLoadModule('/src/components/chat/MessageGroup.tsx'))
  ;({ MediaModal } = await server.ssrLoadModule('/src/components/chat/MediaModal.tsx'))
  ;({ RemoteScreen } = await server.ssrLoadModule('/src/components/room/RemoteScreen.tsx'))
  ;({ AccountRow } = await server.ssrLoadModule('/src/components/admin/AccountRow.tsx'))
  ;({ NekoClient } = await server.ssrLoadModule('/src/neko/client.ts'))
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
  return { f, neko, sent, window, listeners, down: (button, buttons) => mouse(overlay, 'mousedown', button, buttons), up: (button, buttons) => mouse(window, 'mouseup', button, buttons) }
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
