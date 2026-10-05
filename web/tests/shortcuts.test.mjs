import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let matchRoomShortcut, shortcuts
try {
  ;({ matchRoomShortcut, shortcuts } = await server.ssrLoadModule('/src/components/room/shortcuts.ts'))
} finally { await server.close() }

const context = { enabled: true, host: false, editing: false, dialog: false, kicked: false, inChat: false, range: false }
const match = (key, state = {}, event = {}) => matchRoomShortcut({ key, ctrlKey: false, metaKey: false, altKey: false, repeat: false, ...event }, { ...context, ...state })

test('every table key maps to its action, including uppercase and lowercase letters', () => {
  for (const { action, keys } of shortcuts) for (const key of keys) {
    assert.equal(match(key), action)
    if (key.length === 1) assert.equal(match(key.toLowerCase()), action)
  }
  for (const key of ['Escape', 'Tab', 'ArrowLeft', 'ArrowRight', '/', ' ', 'Unidentified']) assert.equal(match(key), null)
  assert.equal(match('?', {}, { shiftKey: true }), 'settings')
})

for (const state of [{ enabled: false }, { host: true }, { editing: true }, { dialog: true }, { kicked: true }]) {
  test(`all shortcuts pause with ${JSON.stringify(state)}`, () => {
    for (const { keys } of shortcuts) assert.equal(match(keys[0], state), null)
  })
}

for (const modifier of ['ctrlKey', 'metaKey', 'altKey']) {
  test(`${modifier} leaves keys to the browser`, () => {
    for (const { keys } of shortcuts) assert.equal(match(keys[0], {}, { [modifier]: true }), null)
  })
}

test('only volume arrows repeat, and they leave chat scrolling and range controls alone', () => {
  for (const { action, keys } of shortcuts) {
    const arrow = action === 'volumeUp' || action === 'volumeDown'
    assert.equal(match(keys[0], {}, { repeat: true }), arrow ? action : null)
    for (const state of [{ inChat: true }, { range: true }]) assert.equal(match(keys[0], state), arrow ? null : action)
  }
})
