import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

// Exercise component handlers and effect cleanup with browser APIs supplied
// by each fixture, using the same Vite transform as the other Node tests.
const mocks = {
  'preact/compat': `export const createPortal = (node, container) => { globalThis.frontendFixture.portal = container; return node }`,
  'preact/hooks': `
    export const useState = (...args) => globalThis.frontendFixture.state(...args)
    export const useRef = (...args) => globalThis.frontendFixture.ref(...args)
    export const useEffect = (...args) => globalThis.frontendFixture.effect(...args)
    export const useLayoutEffect = useEffect
    export const useCallback = fn => fn
    export const useContext = () => globalThis.frontendFixture.store ?? null
    export const useMemo = (...args) => globalThis.frontendFixture.memo(...args)
    export const useId = () => 'test-dialog'
  `,
  'cropperjs': `export default class {
    constructor() { globalThis.frontendFixture.cropper = this }
    getCroppedCanvas() { return globalThis.frontendFixture.canvas }
    destroy() {}
  }`,
  '/app/state': `
    export const preferences = { get value() { return globalThis.frontendFixture?.preferences ?? { volume: 100, muted: false } } }
    export const me = { get value() { return globalThis.frontendFixture?.me ?? null }, set value(user) { globalThis.frontendFixture.me = user } }
    export const settingsOpen = { get value() { return globalThis.frontendFixture.settingsOpen }, set value(section) { globalThis.frontendFixture.settingsOpen = section } }
    export const logout = async () => globalThis.frontendFixture.logout()
    export const serverSettings = { value: { registration: 'open' } }
    export const resolveTheme = theme => theme
    export const updatePreferences = () => {}
  `,
  '/RoomContext': `export const useRoomStore = () => globalThis.frontendFixture.store; export const RoomContext = {}`,
  'react-colorful': `export function HexColorInput() {}; export function HexColorPicker() {}`,
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
let readableNameColor, cropKeyboard, useDialogFocus, Button, ButtonLink, Modal, Input, FormActions, AvatarChooser, ProfileEditor, SettingsDialog, profileChanged, userIdentity, ChatPanel, MessageGroup, MessageList, ChatPreview, MediaModal, RemoteScreen, AccountRow, NekoClient, KickedScreen, DesktopUploadStatus, PermissionRow, PermissionFields, PermissionTable, BanDate, Notice, blankPermission, config
try {
  ;({ readableNameColor } = await server.ssrLoadModule('/src/components/chat/nameColor.ts'))
  ;({ cropKeyboard } = await server.ssrLoadModule('/src/components/ui/cropKeyboard.ts'))
  ;({ useDialogFocus } = await server.ssrLoadModule('/src/components/ui/useDialogFocus.ts'))
  ;({ Button, ButtonLink } = await server.ssrLoadModule('/src/components/Button.tsx'))
  ;({ Modal } = await server.ssrLoadModule('/src/components/Modal.tsx'))
  ;({ Input } = await server.ssrLoadModule('/src/components/ui/Field.tsx'))
  ;({ FormActions } = await server.ssrLoadModule('/src/components/ui/FormActions.tsx'))
  ;({ AvatarChooser } = await server.ssrLoadModule('/src/components/profile/AvatarChooser.tsx'))
  ;({ ProfileEditor } = await server.ssrLoadModule('/src/components/profile/ProfileEditor.tsx'))
  ;({ SettingsDialog } = await server.ssrLoadModule('/src/components/settings/SettingsDialog.tsx'))
  ;({ profileChanged } = await server.ssrLoadModule('/src/components/profile/profileChanges.ts'))
  ;({ userIdentity } = await server.ssrLoadModule('/src/components/room/UserHoverName.ts'))
  ;({ ChatPanel } = await server.ssrLoadModule('/src/components/chat/ChatPanel.tsx'))
  ;({ MessageGroup } = await server.ssrLoadModule('/src/components/chat/MessageGroup.tsx'))
  ;({ MessageList } = await server.ssrLoadModule('/src/components/chat/MessageList.tsx'))
  ;({ ChatPreview } = await server.ssrLoadModule('/src/components/settings/ChatPreview.tsx'))
  ;({ MediaModal } = await server.ssrLoadModule('/src/components/chat/MediaModal.tsx'))
  ;({ PermissionRow, blankPermission } = await server.ssrLoadModule('/src/components/admin/PermissionRow.tsx'))
  ;({ PermissionFields } = await server.ssrLoadModule('/src/components/admin/PermissionFields.tsx'))
  ;({ PermissionTable } = await server.ssrLoadModule('/src/components/admin/PermissionTable.tsx'))
  ;({ BanDate } = await server.ssrLoadModule('/src/components/admin/BanDate.tsx'))
  ;({ Notice } = await server.ssrLoadModule('/src/components/ui/Notice.tsx'))
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
      slots[i] ??= { value: typeof value === 'function' ? value() : value }
      return [slots[i].value, (next) => { slots[i].value = typeof next === 'function' ? next(slots[i].value) : next }]
    },
    memo(fn, deps) {
      const i = index++
      if (!slots[i] || deps.some((dep, j) => dep !== slots[i].deps[j])) slots[i] = { value: fn(), deps }
      return slots[i].value
    },
    ref(value) {
      const i = index++
      slots[i] ??= { current: value }
      return slots[i]
    },
    effect(fn, deps) {
      const i = index++
      if (!slots[i] || !deps || deps.some((dep, j) => dep !== slots[i].deps[j])) {
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

test('profile dirtiness compares nickname, colour and a pending avatar', () => {
  const user = { nickname: 'Alice', nameColor: '#FF9900' }
  assert.equal(profileChanged(user, 'Alice', '#ff9900', null), false)
  assert.equal(profileChanged(user, 'Alice', '#f90', null), false)
  assert.equal(profileChanged(user, 'Alicia', '#ff9900', null), true)
  assert.equal(profileChanged(user, 'Alice', '#ffffff', null), true)
  assert.equal(profileChanged(user, 'Alice', '#ff9900', new Blob(['avatar'])), true)
})

for (const action of ['section', 'close', 'logout']) {
  test(`personal settings guard ${action} only while the profile is dirty`, async (t) => {
    const f = fixture(t)
    f.settingsOpen = 'account'
    f.me = { username: 'alice', nickname: 'Alice' }
    let closed = 0, loggedOut = 0
    f.logout = () => { loggedOut++ }
    const render = () => f.render(SettingsDialog, { onClose: () => closed++ })
    const request = () => {
      const window = named(render(), 'SettingsWindow')
      if (action === 'close') window.props.onClose()
      else window.props.onSelect(action === 'logout' ? 'logout' : 'appearance')
    }
    const dirty = (value) => named(render(), 'AccountSection').props.onDirtyChange(value)
    dirty(true)
    named(render(), 'SettingsWindow').props.onSelect('account')
    assert.equal(named(render(), 'Modal'), undefined, 'the current section keeps editing')
    request()
    assert.equal(f.settingsOpen, 'account')
    assert.equal(closed + loggedOut, 0)
    let modal = named(render(), 'Modal')
    assert.equal(modal.props.title, 'Discard profile changes?')
    button(modal.props.footer, 'Keep editing').props.onClick()
    assert.equal(named(render(), 'Modal'), undefined)
    request()
    named(render(), 'Modal').props.onClose()
    assert.equal(f.settingsOpen, 'account', 'Escape/backdrop on the question keeps editing')
    request()
    modal = named(render(), 'Modal')
    button(modal.props.footer, 'Discard').props.onClick()
    await Promise.resolve()
    assert.equal(named(render(), 'Modal'), undefined)
    assert.equal(f.settingsOpen, action === 'section' ? 'appearance' : action === 'close' ? null : 'account')
    assert.equal(closed, action === 'close' ? 1 : 0)
    assert.equal(loggedOut, action === 'logout' ? 1 : 0)

    f.settingsOpen = 'account'
    dirty(false)
    request()
    await Promise.resolve()
    assert.equal(named(render(), 'Modal'), undefined, 'clean or saved profiles leave immediately')
    assert.equal(closed, action === 'close' ? 2 : 0)
    assert.equal(loggedOut, action === 'logout' ? 2 : 0)
  })
}

test('profile edits, reset and successful saves report the current dirty state', async (t) => {
  const f = fixture(t), changes = [], requests = []
  f.me = { username: 'alice', nickname: 'Alice', nameColor: '#f90', avatarUrl: '' }
  globals(t, { fetch: async (path, init) => {
    requests.push([path, init.method])
    const user = path.endsWith('/avatar') ? { ...f.me, avatarUrl: '/media/avatars/alice.png' }
      : { ...f.me, nickname: JSON.parse(init.body).nickname.trim(), nameColor: '#ff9900' }
    return { ok: true, status: 200, json: async () => ({ user }) }
  } })
  const props = { onDirtyChange: (dirty) => changes.push(dirty) }
  const render = () => f.render(ProfileEditor, props)
  render()
  render()
  assert.equal(changes.at(-1), false)
  nodes(render()).find((n) => n.type?.name === 'Input').props.onInput({ currentTarget: { value: ' Alicia ' } })
  render()
  assert.equal(changes.at(-1), true)
  button(render(), 'Reset').props.onClick()
  render()
  assert.equal(changes.at(-1), false)
  nodes(render()).find((n) => n.type?.name === 'Input').props.onInput({ currentTarget: { value: ' Alicia ' } })
  named(render(), 'AvatarChooser').props.onCrop(new Blob(['avatar']))
  render()
  assert.equal(changes.at(-1), true)
  render().props.onSubmit({ preventDefault() {} })
  await new Promise((resolve) => setImmediate(resolve))
  const saved = render()
  assert.equal(changes.at(-1), false)
  assert.equal(f.me.nickname, 'Alicia', 'the form adopts the saved nickname')
  assert.equal(button(saved, 'Save changes').props.disabled, true)
  assert.deepEqual(requests, [['/api/me', 'PATCH'], ['/api/me/avatar', 'POST']])
})

test('edits made during a profile save stay dirty afterwards', async (t) => {
  const f = fixture(t), changes = []
  f.me = { username: 'alice', nickname: 'Alice', nameColor: '#f90', avatarUrl: '' }
  let finish
  globals(t, { fetch: () => new Promise((resolve) => { finish = resolve }) })
  const render = () => f.render(ProfileEditor, { onDirtyChange: (dirty) => changes.push(dirty) })
  const input = () => named(render(), 'Input')
  render()
  render()
  input().props.onInput({ currentTarget: { value: 'Alicia' } })
  render().props.onSubmit({ preventDefault() {} })
  input().props.onInput({ currentTarget: { value: 'Ally' } })
  finish({ ok: true, status: 200, json: async () => ({ user: { ...f.me, nickname: 'Alicia' } }) })
  await new Promise((resolve) => setImmediate(resolve))
  assert.equal(input().props.value, 'Ally')
  assert.equal(changes.at(-1), true)
})

for (const failed of ['/api/me', '/api/me/avatar']) {
  test(`a failed ${failed} save keeps the profile dirty`, async (t) => {
    const f = fixture(t), changes = []
    f.me = { username: 'alice', nickname: 'Alice', nameColor: '#f90', avatarUrl: '' }
    globals(t, { fetch: async (path, init) => path === failed
      ? { ok: false, status: 500, json: async () => ({ error: 'Save failed.' }) }
      : { ok: true, status: 200, json: async () => ({ user: { ...f.me, nickname: JSON.parse(init.body).nickname } }) }
    })
    const props = { onDirtyChange: (dirty) => changes.push(dirty) }
    const render = () => f.render(ProfileEditor, props)
    render()
    render()
    named(render(), 'Input').props.onInput({ currentTarget: { value: 'Alicia' } })
    named(render(), 'AvatarChooser').props.onCrop(new Blob(['avatar']))
    render().props.onSubmit({ preventDefault() {} })
    await new Promise((resolve) => setImmediate(resolve))
    const node = render()
    assert.equal(changes.at(-1), true)
    assert.equal(button(node, 'Save changes').props.disabled, false)
    assert.equal(named(node, 'Notice').props.tone, 'error')
  })
}

test('the visible Change avatar button opens the same chooser as the picture', (t) => {
  const f = fixture(t)
  let opened = 0
  f.elements = { input: { click() { opened++ } } }
  const props = { avatar: '', disabled: false, onCrop() {} }
  const node = f.render(AvatarChooser, props)
  const change = button(node, 'Change avatar')
  assert.equal(change.type.name, 'Button')
  change.props.onClick()
  nodes(node).find((n) => n.type === 'button').props.onClick()
  assert.equal(opened, 2)
  const disabled = f.render(AvatarChooser, { ...props, disabled: true })
  assert.equal(button(disabled, 'Change avatar').props.disabled, true)
  assert.equal(nodes(disabled).find((n) => n.type === 'button').props.disabled, true)
})

test('anonymous identities use one spelling, including messages after the author leaves', (t) => {
  const f = fixture(t)
  const user = { key: 'a:abcd1234', anonymous: true, username: '' }
  assert.equal(userIdentity(user), 'Anon(abcd)')
  assert.equal(userIdentity(user.key), userIdentity(user))
  assert.equal(userIdentity({ key: 'u:1', anonymous: false, username: 'alice' }), 'alice')
  f.store = { selfKey: { value: 'u:1' }, users: { value: new Map() }, rights: { value: { admin: false } } }
  f.preferences = { chatStyle: 'modern', chatAvatars: false }
  const message = { id: 1, author: user.key, anonymous: true, nickname: 'Guest', type: 'text', body: 'Hello', time: 0 }
  const node = f.render(MessageGroup, { messages: [message], editing: null })
  assert.ok(nodes(node).find((n) => n.props?.title === userIdentity(user)))
})

test('compact splits consecutive messages and repeats each message name and timestamp', (t) => {
  const f = fixture(t)
  globals(t, { window: new EventTarget(), ResizeObserver: class { observe() {} disconnect() {} } })
  const messages = [
    { id: 1, author: 'u:2', nickname: 'Bob', nameColor: '#4aa', type: 'text', body: 'First', time: new Date(2026, 0, 1, 13, 5).getTime() },
    { id: 2, author: 'u:2', nickname: 'Robert', nameColor: '#fff', type: 'text', body: 'Second', time: new Date(2026, 0, 1, 13, 6).getTime() },
    { id: 3, author: 'u:2', nickname: 'Robert', nameColor: '#fff', type: 'text', body: 'Third', time: new Date(2026, 0, 1, 13, 7).getTime() },
  ]
  const store = {
    chat: { value: messages }, selfKey: { value: 'u:1' },
    users: { value: new Map([['u:2', { username: 'bob' }]]) }, rights: { value: { admin: false } },
  }
  f.store = store
  const props = { lines: [{ id: 'join', after: 2, body: 'Pixel joined', time: messages[1].time }], editing: null }
  let compactGroups
  for (const chatStyle of ['classic', 'modern', 'compact', 'classic']) {
    f.preferences = { chatStyle, showLeaveJoinMsg: true }
    const node = f.render(MessageList, props)
    const groups = nodes(node).filter((n) => n.type === MessageGroup)
    assert.deepEqual(groups.map((g) => g.props.messages.map((m) => m.id)), chatStyle === 'compact' ? [[1], [2], [3]] : [[1, 2], [3]])
    const entries = node.props.children[0].props.children
    assert.equal(entries[chatStyle === 'compact' ? 2 : 1].props.children[1].props.children, 'Pixel joined', 'join line stays after the second message')
    if (chatStyle === 'compact') compactGroups = groups
  }
  const groupFixture = fixture(t)
  groupFixture.store = store
  groupFixture.preferences = { chatStyle: 'compact', chatAvatars: true }
  for (const [index, group] of compactGroups.entries()) {
    const node = groupFixture.render(MessageGroup, group.props)
    const header = nodes(node).find((n) => n.props?.title === 'bob')
    assert.equal(header.props.children[0], messages[index].nickname)
    assert.equal(nodes(header).find((n) => n.type === 'span').props.children, ['1:05 PM', '1:06 PM', '1:07 PM'][index])
    assert.equal(named(node, 'ChatAvatar'), undefined)
    assert.equal(named(node, 'MessageText').props.body, messages[index].body)
  }
})

test('chat keeps message avatars after the author leaves and display preferences change', (t) => {
  const f = fixture(t)
  const message = { id: 1, author: 'u:2', nickname: 'Bob', nameColor: '#4aa', type: 'text', body: 'Hello', time: 0, avatarUrl: '/media/avatars/bob.png' }
  f.store = { selfKey: { value: 'u:1' }, users: { value: new Map([['u:2', { username: 'bob', avatarUrl: message.avatarUrl }]]) }, rights: { value: { admin: false } } }
  f.preferences = { chatStyle: 'modern', chatAvatars: true }
  const render = () => f.render(MessageGroup, { messages: [message], editing: null })
  assert.equal(named(render(), 'ChatAvatar').props.url, message.avatarUrl)
  f.store.users.value = new Map()
  assert.equal(named(render(), 'ChatAvatar').props.url, message.avatarUrl)
  f.preferences.chatStyle = 'compact'
  assert.equal(named(render(), 'ChatAvatar'), undefined)
  f.preferences.chatStyle = 'classic'
  assert.equal(named(render(), 'ChatAvatar').props.url, message.avatarUrl)
  f.preferences.manualLoadMedia = true
  assert.equal(named(render(), 'ChatAvatar').props.url, undefined)
  f.preferences.manualLoadMedia = false
  assert.equal(named(render(), 'ChatAvatar').props.url, message.avatarUrl)
  f.store.users.value = new Map([['u:2', { username: 'bob', avatarUrl: '/media/avatars/new.png' }]])
  assert.equal(named(render(), 'ChatAvatar').props.url, '/media/avatars/new.png', 'online profile updates override the message avatar')
})

test('the compact preview repeats the name and timestamp for every sample message', (t) => {
  const f = fixture(t)
  for (const chatStyle of ['classic', 'modern', 'compact']) {
    f.preferences = { chatStyle, chatScale: 100, showLeaveJoinMsg: false }
    const node = ChatPreview({ nickname: 'Alice' })
    const headers = nodes(node).filter((n) => n.props?.style?.['--name-colour'])
    assert.deepEqual(headers.map((n) => n.props.children[0]), chatStyle === 'compact' ? ['Mochi', 'Alice', 'Alice'] : ['Mochi', 'Alice'])
    assert.deepEqual(headers.map((n) => nodes(n).find((child) => child.type === 'span').props.children), chatStyle === 'compact' ? ['8:57 PM', '8:58 PM', '8:58 PM'] : ['8:57 PM', '8:58 PM'])
  }
})

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
  const toggle = (node, name) => nodes(node).find((n) => n.type?.name === 'Switch' && n.props.label === `${name}: alice`)
  let node = AccountRow(props)
  assert.equal(toggle(node, 'Verified').props.disabled, false)
  toggle(node, 'Verified').props.onChange(false)
  assert.deepEqual(changes, [['alice', { verified: false }]])
  for (const name of ['Admin', 'Enabled']) assert.equal(toggle(node, name).props.disabled, true)
  const action = (node, label) => nodes(node).find((n) => n.props?.['aria-label'] === label)
  for (const label of ['Delete alice', 'Reset password of alice']) assert.equal(action(node, label).props.disabled, true)
  node = AccountRow({ ...props, busy: true })
  assert.equal(toggle(node, 'Verified').props.disabled, true)
})

test('media clicks select the message ID and the modal reads its current URL', (t) => {
  const f = fixture(t), opened = []
  f.store = { selfKey: { value: 'u:1' }, users: { value: new Map() }, rights: { value: { admin: false } } }
  for (const type of ['image', 'video']) {
    const message = { id: 7, author: 'u:2', type, mediaUrl: '/media/chat/current', time: 0 }
    const node = f.render(MessageGroup, { messages: [message], editing: null, onMedia: (id) => opened.push(id) })
    named(node, 'InlineMedia').props.onOpen({ type, url: '/stale-url' })
  }
  assert.deepEqual(opened, [7, 7])
})

test('the media preview is the bare picture or video; a click outside the player closes it', (t) => {
  const f = fixture(t)
  focusDom(t, f)
  for (const type of ['image', 'video']) {
    let closed = 0
    const message = { id: 7, author: 'u:2', type, mediaUrl: '/media/chat/current', time: 0 }
    const modal = f.render(MediaModal, { message, onClose: () => closed++ })
    const media = nodes(modal).find((n) => n.type === (type === 'image' ? 'img' : 'video'))
    assert.equal(media.props.src, message.mediaUrl)
    // No frame: no title, close button or extra link around it.
    assert.deepEqual(nodes(modal).map((n) => n.type), type === 'image' ? ['div', 'div', 'a', 'img'] : ['div', 'div', 'video'])
    modal.props.onClick()
    assert.equal(closed, 1)
    if (type === 'video') {
      let stopped = false
      media.props.onClick({ stopPropagation() { stopped = true } })
      assert.equal(stopped, true, 'the player\'s controls must not close the preview')
    }
    globalThis.document.dispatchEvent(Object.assign(new Event('keydown'), { key: 'Escape' }))
    assert.equal(closed, 2)
  }
})

function focusDom(t, f) {
  const document = new EventTarget()
  class Element {
    offsetParent = {}
    children = []
    focus() { document.activeElement = this }
    querySelectorAll() { return this.children }
    getAttribute(name) { return this[name] ?? null }
  }
  const previous = new Element(), dialog = new Element()
  previous.focus()
  document.body = new Element()
  globals(t, { document, HTMLElement: Element })
  f.elements = { div: dialog }
  return { document, Element, previous, dialog }
}

for (const type of ['image', 'video']) {
  test(`${type} preview traps Tab in both directions, closes on Escape and restores focus`, (t) => {
    const f = fixture(t), { document, Element, previous, dialog } = focusDom(t, f)
    document.fullscreenElement = new Element()
    const first = new Element(), last = new Element(), hidden = new Element()
    hidden.offsetParent = null
    dialog.children = [first, last, hidden]
    let closed = 0
    const node = f.render(MediaModal, { message: { type }, onClose: () => closed++ })
    assert.equal(f.portal, document.fullscreenElement, 'the portal escapes the chat stacking context')
    assert.equal(node.props['aria-modal'], 'true')
    assert.equal(document.activeElement, first)
    dialog.focus()
    const key = (key, shiftKey = false) => {
      const event = Object.assign(new Event('keydown', { cancelable: true }), { key, shiftKey })
      document.dispatchEvent(event)
      return event
    }
    assert.equal(key('Tab').defaultPrevented, true)
    assert.equal(document.activeElement, first)
    assert.equal(key('Tab', true).defaultPrevented, true)
    assert.equal(document.activeElement, last)
    key('Tab')
    assert.equal(document.activeElement, first)
    dialog.focus()
    key('Tab', true)
    assert.equal(document.activeElement, last)
    dialog.children = []
    dialog.focus()
    assert.equal(key('Tab').defaultPrevented, true, 'a preview without loaded controls still traps focus')
    assert.equal(key('Escape').defaultPrevented, true)
    assert.equal(closed, 1)
    f.unmount()
    assert.equal(document.activeElement, previous)
    key('Escape')
    assert.equal(closed, 1, 'unmount removes the keyboard listener')
    // The fixture also unmounts in teardown.
    f.unmount = () => {}
  })
}

test('compact notices preserve announcement roles for all tones', () => {
  for (const tone of ['error', 'success', 'info']) {
    for (const compact of [false, true]) {
      const node = Notice({ tone, compact, children: 'Message' })
      assert.equal(node.props.role, tone === 'error' ? 'alert' : 'status')
      assert.equal(node.props.class.includes('compact'), compact)
      assert.equal(nodes(node).find((n) => n.type === 'span').props.children, 'Message')
    }
  }
})

test('avatar validation uses a compact error notice', (t) => {
  const f = fixture(t)
  const render = () => f.render(AvatarChooser, { avatar: '', onCrop() {} })
  nodes(render()).find((n) => n.type === 'input').props.onChange({ currentTarget: { files: [new Blob([], { type: 'text/plain' })], value: '' } })
  const notice = named(render(), 'Notice')
  assert.equal(notice.props.compact, true)
  assert.equal(notice.props.tone, 'error')
  assert.match(notice.props.children, /PNG, JPEG or WebP/)
})

test('permission creation has labelled fields above the table in admin and room settings', async (t) => {
  for (const room of [undefined, 'default']) {
    const f = fixture(t)
    globals(t, { fetch: async (path) => ({ ok: true, status: 200, json: async () => path === '/api/rooms' ? [{ name: 'default' }] : [] }) })
    const render = () => f.render(PermissionTable, { room })
    render()
    await new Promise((resolve) => setImmediate(resolve))
    const node = render(), create = named(node, 'PermissionRow'), table = named(node, 'AdminTable')
    assert.equal(create.props.creating, true)
    assert.equal(named(table, 'PermissionRow'), undefined)
    assert.ok(nodes(node).indexOf(create) < nodes(node).indexOf(table))
    const fields = PermissionFields({ draft: blankPermission('default'), room, rooms: [], creating: true, busy: false, until: '', onChange() {}, onUntil() {} })
    assert.equal(nodes(fields).some((n) => n.type === 'td'), false)
    const labels = nodes(fields).filter((n) => n.type?.name === 'Field').map((n) => n.props.label)
    assert.deepEqual(labels, [...(room ? [] : ['Room']), 'User', 'Remote', 'Images', 'Upload', 'Trusted', 'Invited', 'Invite name'])
  }
})

test('permission toolbar adds, clears and preserves ban date saving', async (t) => {
  const f = fixture(t), requests = [], saved = []
  const permission = blankPermission('default')
  const props = { permission, room: 'default', rooms: [], creating: true, onSaved: (value) => saved.push(value), onDeleted() {} }
  globals(t, { fetch: async (path, init) => {
    const body = JSON.parse(init.body)
    requests.push({ path, body })
    return { ok: true, status: 200, json: async () => ({ ...permission, username: 'alice', ...body }) }
  } })
  const render = () => f.render(PermissionRow, props)
  const change = (value) => named(render(), 'PermissionFields').props.onChange(value)
  assert.equal(render().type, 'form')
  assert.equal(render().props['aria-label'], 'Add permission')
  change({ username: 'alice', remote: true })
  button(render(), 'Clear').props.onClick()
  assert.equal(named(render(), 'PermissionFields').props.draft.username, '')
  for (const [banned, until] of [[true, '2026-11-01T12:30'], [true, ''], [false, '2026-11-01T12:30']]) {
    change({ username: ' alice ', banned, remote: true, inviteName: 'Guest' })
    named(render(), 'PermissionFields').props.onUntil(until)
    render().props.onSubmit({ preventDefault() {} })
    await new Promise((resolve) => setImmediate(resolve))
    assert.equal(requests.at(-1).path, '/api/admin/permissions/default/alice')
    assert.equal(requests.at(-1).body.bannedUntil, banned && until ? new Date(until).getTime() / 1000 : null)
    assert.equal(requests.at(-1).body.remote, true)
    assert.equal(requests.at(-1).body.inviteName, 'Guest')
    assert.equal(named(render(), 'PermissionFields').props.draft.username, '')
    assert.equal(named(render(), 'Notice').props.compact, true)
    assert.equal(named(render(), 'Notice').props.children, 'Permission added.')
  }
  assert.equal(saved.length, 3)
  render().props.onSubmit({ preventDefault() {} })
  assert.equal(named(render(), 'Notice').props.tone, 'error')
})

test('ban date chip reveals a labelled editor and returns focus on Done or Escape', (t) => {
  const f = fixture(t), updates = []
  let focused = 0
  f.elements = { button: { focus() { focused++ } } }
  const props = { banned: false, until: '', busy: false, name: 'alice', onUntil: (value) => updates.push(value) }
  const render = () => f.render(BanDate, props)
  const chip = () => nodes(render()).find((n) => n.type === 'button')
  assert.equal(chip().props.children, 'No ban')
  assert.equal(named(render(), 'Input'), undefined)
  chip().props.onClick()
  assert.equal(chip().props['aria-expanded'], true)
  const input = named(render(), 'Input')
  assert.match(input.props['aria-label'], /Banned until for alice/)
  assert.equal(input.props.autoFocus, true)
  input.props.onInput({ currentTarget: { value: '2026-11-01T12:30' } })
  assert.deepEqual(updates, ['2026-11-01T12:30'])
  button(render(), 'Done').props.onClick()
  assert.equal(named(render(), 'Input'), undefined)
  assert.equal(focused, 1)
  props.banned = true
  assert.equal(chip().props.children, 'Forever')
  props.until = '2026-11-01T12:30'
  assert.equal(chip().props.children, '2026-11-01 12:30')
  chip().props.onClick()
  nodes(render()).find((n) => n.props?.onKeyDown).props.onKeyDown({ key: 'Escape', preventDefault() {}, stopPropagation() {} })
  assert.equal(named(render(), 'Input'), undefined)
  assert.equal(focused, 2)
  props.busy = true
  assert.equal(chip().props.disabled, true)
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

test('kick screens share the site header and page buttons', (t) => {
  const f = fixture(t)
  f.store = { kicked: { value: { reason: 'not_found' } } }
  const node = KickedScreen()
  assert.ok(named(node, 'Header'))
  assert.equal(named(node, 'InfoScreen').props.message, 'Room not found')
  const home = button(node, 'Back to rooms')
  assert.equal(home.type.name, 'ButtonLink')
  assert.equal(home.props.href, '/')
  assert.equal(home.props.variant, 'primary')
  for (const reason of ['banned', 'account', 'verified', 'invite', 'kicked', 'deleted', 'not_found', 'session']) {
    f.store.kicked.value.reason = reason
    const screen = KickedScreen()
    assert.ok(named(screen, 'Header'))
    assert.ok(button(screen, 'Back to rooms'))
    const login = button(screen, 'Log in')
    if (reason === 'account' || reason === 'session') {
      assert.equal(login.type.name, 'ButtonLink')
      assert.equal(login.props.href, '/login')
    } else assert.equal(login, undefined)
    if (reason === 'session') assert.equal(named(screen, 'InfoScreen').props.message, 'Session expired')
  }
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


test('shared button variants consume styling props and preserve actions and links', () => {
  let clicked = 0
  const node = Button({ variant: 'danger-ghost', size: 'sm', icon: 'trash', children: undefined, onClick: () => clicked++ })
  assert.equal(node.type, 'button')
  assert.equal(node.props.type, 'button')
  assert.equal(node.props.variant, undefined)
  assert.equal(node.props.size, undefined)
  node.props.onClick()
  assert.equal(clicked, 1)
  assert.equal(Button({ variant: 'primary', type: 'submit', children: 'Save' }).props.type, 'submit')
  const link = ButtonLink({ variant: 'primary', href: '/room/default', children: 'Join' })
  assert.equal(link.type, 'a')
  assert.equal(link.props.href, '/room/default')
  assert.equal(link.props.children[1], 'Join')
  assert.equal(link.props.variant, undefined)
})

test('modal sizes retain the labelled dialog and close action', () => {
  let closed = 0
  for (const size of ['sm', 'md', 'lg', 'xl']) {
    const node = Modal({ title: 'Question', size, onClose: () => closed++, children: 'Content' })
    assert.equal(node.props.labelledBy, 'test-dialog')
    const title = nodes(node).find((n) => n.type === 'h2')
    assert.equal(title.props.id, node.props.labelledBy)
    assert.equal(title.props.children, 'Question')
    named(node, 'CloseButton').props.onClick()
  }
  assert.equal(closed, 4)
})

test('quiet inputs consume the appearance prop and retain normal input events', () => {
  const values = []
  const node = Input({ compact: true, quiet: true, value: 'Friends', onInput: (e) => values.push(e.currentTarget.value) })
  assert.equal(node.props.quiet, undefined)
  assert.equal(node.props.compact, undefined)
  assert.equal(node.props.value, 'Friends')
  node.props.onInput({ currentTarget: { value: 'Movie night' } })
  assert.deepEqual(values, ['Movie night'])
})

test('settings form feedback keeps both notices and their announcement roles', () => {
  const node = FormActions({ error: 'Save failed.', message: 'Saved.', children: 'Save button' })
  const notices = nodes(node).filter((n) => n.type === Notice)
  assert.deepEqual(notices.map((n) => n.props.children), ['Save failed.', 'Saved.'])
  assert.deepEqual(notices.map((n) => Notice(n.props).props.role), ['alert', 'status'])
  assert.equal(nodes(FormActions({ children: 'Save button' })).filter((n) => n.type === Notice).length, 0)
})


test('name colours keep readable choices and meet AA on both bubble and hover surfaces', () => {
  const rgb = (color) => color.startsWith('#')
    ? (color.length === 4 ? [...color.slice(1)].map(c => parseInt(c + c, 16)) : [1, 3, 5].map(i => parseInt(color.slice(i, i + 2), 16)))
    : color.match(/\d+/g).map(Number)
  const luminance = (color) => rgb(color).map(v => v / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4).reduce((sum, v, i) => sum + v * [.2126, .7152, .0722][i], 0)
  for (const [base, hover] of [['#1e2024', '#292b2f'], ['#fdfdfd', '#e4e4e4'], ['#595959', '#000']]) {
    for (const color of ['#000', '#fff', '#f90', '#4aa', '#00f', '#f00', '#080', '#ff0']) {
      const result = readableNameColor(color, base, hover)
      for (const bg of [base, hover]) {
        const a = luminance(result), b = luminance(bg)
        assert((Math.max(a, b) + .05) / (Math.min(a, b) + .05) >= 4.6, `${color} against ${bg}`)
      }
    }
  }
  assert.equal(readableNameColor('#f90', '#1e2024'), '#f90')
  assert.equal(readableNameColor('#000', '#fdfdfd'), '#000')
  assert.equal(readableNameColor('invalid', '#fff'), 'invalid')
})

test('crop keyboard moves, resizes, preserves square avatars and leaves Escape to the dialog', () => {
  let data = { x: 20, y: 30, width: 100, height: 100 }, prevented = 0, stopped = 0
  const cropper = { getData: () => data, setData: update => { data = { ...data, ...update } } }
  const key = (key, shiftKey = false, square = false) => cropKeyboard({ key, shiftKey, preventDefault() { prevented++ }, stopPropagation() { stopped++ } }, cropper, square)
  key('ArrowRight'); key('ArrowUp')
  assert.deepEqual(data, { x: 30, y: 20, width: 100, height: 100 })
  key('ArrowRight', true); key('ArrowDown', true)
  assert.deepEqual(data, { x: 30, y: 20, width: 110, height: 110 })
  key('ArrowLeft', true, true)
  assert.deepEqual(data, { x: 30, y: 20, width: 100, height: 100 })
  data.width = data.height = 1; key('ArrowUp', true, true)
  assert.equal(data.width, 1); assert.equal(data.height, 1)
  key('Escape')
  assert.equal(prevented, 6); assert.equal(stopped, 6)
})

test('dialog wrapping ignores roving items and unchecked members of a native radio group', (t) => {
  const f = fixture(t), { document, Element, previous, dialog } = focusDom(t, f)
  const first = new Element(), selected = new Element(), unchecked = new Element(), roving = new Element()
  selected.type = unchecked.type = 'radio'; selected.name = unchecked.name = 'theme'; selected.checked = true
  roving.tabIndex = -1
  dialog.children = [first, selected, unchecked, roving]
  f.render(() => ({ type: 'div', ref: useDialogFocus('radio-window', () => {}) }))
  assert.equal(document.activeElement, first)
  selected.focus()
  const tab = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Tab' })
  document.dispatchEvent(tab)
  assert(tab.defaultPrevented); assert.equal(document.activeElement, first)
  const back = Object.assign(new Event('keydown', { cancelable: true }), { key: 'Tab', shiftKey: true })
  document.dispatchEvent(back)
  assert(back.defaultPrevented); assert.equal(document.activeElement, selected)
  f.unmount(); assert.equal(document.activeElement, previous); f.unmount = () => {}
})


test('Escape closes only the top dialog and each close restores its opener', (t) => {
  const parent = fixture(t), { document, Element, previous, dialog } = focusDom(t, parent)
  const opener = new Element(); dialog.children = [opener]
  let parentClosed = 0, childClosed = 0
  parent.render(() => ({ type: 'div', ref: useDialogFocus('parent', () => parentClosed++) }))
  const child = fixture(t), childDialog = new Element()
  child.elements = { div: childDialog }; childDialog.children = [new Element()]
  child.render(() => ({ type: 'div', ref: useDialogFocus('child', () => childClosed++) }))
  const escape = () => document.dispatchEvent(Object.assign(new Event('keydown', { cancelable: true }), { key: 'Escape' }))
  escape(); assert.equal(childClosed, 1); assert.equal(parentClosed, 0)
  child.unmount(); child.unmount = () => {}; assert.equal(document.activeElement, opener)
  escape(); assert.equal(childClosed, 1); assert.equal(parentClosed, 1)
  parent.unmount(); parent.unmount = () => {}; assert.equal(document.activeElement, previous)
})
