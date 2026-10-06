import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

// Use the project's TypeScript transform; this also works on Node builds
// without native TypeScript stripping and needs no additional test dependency.
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let parser, snapshots, mentions
try {
  parser = await server.ssrLoadModule('/src/components/chat/parseMessage.ts')
  snapshots = await server.ssrLoadModule('/src/components/chat/chatChanges.ts')
  mentions = await server.ssrLoadModule('/src/components/chat/mentionCompletion.ts')
} finally { await server.close() }
const { parseMessage, pingName, pingCount, matchedPingNames, groupMessages, messageTime } = parser
const { chatChanges } = snapshots
const { activeMention, mentionCandidates, completeMention } = mentions

const user = (key, nickname = 'Alice') => ({ key, nickname, username: key.startsWith('u:') ? nickname.toLowerCase() : '', anonymous: key.startsWith('a:') })
const message = (id, author = 'u:2', body = 'Hello') => ({ id, author, body, type: 'text', nickname: 'Bob', nameColor: '#f90', time: 1000 })
const self = user('u:1')
const snapshot = (changes = {}) => ({ chat: [], users: new Map([[self.key, self]]), self, connected: true, ...changes })

test('mention suggestions activate only at the start or after whitespace, including at a middle caret', () => {
  assert.deepEqual(activeMention('@', 1), { start: 0, end: 1, query: '' })
  assert.deepEqual(activeMention('Hi\n\t@Bo', 7), { start: 4, end: 7, query: 'Bo' })
  assert.deepEqual(activeMention('Hi @Bobby later', 5), { start: 3, end: 9, query: 'B' })
  assert.deepEqual(activeMention('Hi @Bobby later', 4), { start: 3, end: 9, query: '' })
  for (const text of ['a@b', 'user@example.org', 'https://example.org/@Bob', '@@Bob', 'hi @Bob ']) {
    assert.equal(activeMention(text, text.length), null, text)
  }
  assert.equal(activeMention('@Bob', 0), null)
  assert.equal(activeMention('@Bob', -1), null)
  assert.equal(activeMention('@Bob', 5), null)
  assert.equal(activeMention('Hi @Bob', 2), null)
  assert.equal(activeMention('Hi @Bob later', 8), null)
})

test('mention candidates exclude self, match nicknames without whitespace, rank prefixes before substrings and cap rows', () => {
  const people = [self, user('u:2', 'Zalice'), user('u:3', 'A lbert'), user('u:4', 'ALIce'), user('u:5', 'Bob')]
  const users = new Map(people.map((person) => [person.key, person]))
  assert.deepEqual(mentionCandidates(users, self.key, 'AL').map((person) => person.nickname), ['A lbert', 'ALIce', 'Zalice'])
  assert.deepEqual(mentionCandidates(users, self.key, 'bo').map((person) => person.key), ['u:5'])
  assert.deepEqual(mentionCandidates(users, self.key, 'absent'), [])
  assert.equal(mentionCandidates(users, self.key, '', 2).length, 2)
  assert.equal(mentionCandidates(users, self.key, '', 0).length, 0)
  const crowd = new Map(Array.from({ length: 12 }, (_, i) => [`u:${i}`, user(`u:${i}`, `Person${i}`)]))
  assert.equal(mentionCandidates(crowd, null, '').length, 8)
  assert.equal(users.size, 5)
})

test('completion replaces the whole token, preserves surrounding text and leaves the caret after a parser-compatible mention', () => {
  const text = 'Hi @Bobby later @Alice'
  const completion = completeMention(text, activeMention(text, 5), 'B ob')
  assert.deepEqual(completion, { text: 'Hi @Bob  later @Alice', caret: 8 })
  assert.equal(pingCount(completion.text, 'B ob'), 1)
  assert(matchedPingNames(new Map([['u:2', user('u:2', 'B ob')]])).has(parseMessage(completion.text).find((part) => part.type === 'ping').target))
  assert.deepEqual(completeMention('@', activeMention('@', 1), 'A l\ti\nc e'), { text: '@Alice ', caret: 7 })
})

test('suggestions offer only forms the existing parser can highlight, including spaced names', () => {
  const people = ['A Friend', 'Bob', 'A@B', 'example.org', ''].map((name, i) => user(`u:${i}`, name))
  const candidates = mentionCandidates(new Map(people.map((person) => [person.key, person])), null, '')
  assert.deepEqual(candidates.map((person) => person.nickname), ['A Friend', 'Bob'])
  for (const person of candidates) {
    const completion = completeMention('@', activeMention('@', 1), person.nickname)
    assert.equal(pingCount(completion.text, person.nickname), 1)
  }
})

test('linkification preserves text/newlines and excludes @ inside URLs and email addresses', () => {
  const body = 'Hello\nhttps://example.org/@Alice?q=1\nuser@example.org @Alice'
  const parts = parseMessage(body)
  assert.equal(parts.map((p) => p.text).join(''), body)
  assert.equal(parts.filter((p) => p.type === 'url').length, 1)
  assert.equal(parts.filter((p) => p.type === 'ping').length, 1)
  assert.equal(pingCount(body, 'Alice'), 1)
})

test('mentions stop at spaces, newlines or the next @; lone @ stays text', () => {
  const body = '@Alice@BOB \n@Alice! @'
  assert.deepEqual(parseMessage(body).filter((p) => p.type === 'ping').map((p) => p.target), ['alice', 'bob', 'alice!'])
  assert.equal(parseMessage(body).map((p) => p.text).join(''), body)
  assert.equal(pingCount(body, 'Alice'), 1)
})

test('each matching mention counts, using nickname without whitespace and ignoring case', () => {
  assert.equal(pingName('A l\ti\nc e'), 'alice')
  assert.equal(pingCount('@ALICE @alice\n@Alice', 'A lice'), 3)
  assert.equal(pingCount('@', ''), 0)
  assert.deepEqual([...matchedPingNames(new Map([['u:1', user('u:1', 'A lice')], ['u:2', user('u:2', 'Bob')]]))], ['alice', 'bob'])
})

test('bare domains are linked while HTML remains plain text', () => {
  const parts = parseMessage('<img onerror=evil()> example.org')
  assert.equal(parts[0].type, 'text')
  assert.equal(parts.at(-1).href, 'http://example.org')
})

test('groups use author keys, retaining the first nickname/colour snapshot', () => {
  const first = message(1, 'u:2')
  const renamed = { ...message(2, 'u:2'), nickname: 'Robert', nameColor: '#fff' }
  const groups = groupMessages([first, renamed, message(3, 'u:3'), message(4, 'u:2')])
  assert.deepEqual(groups.map((g) => g.map((m) => m.id)), [[1, 2], [3], [4]])
  assert.equal(groups[0][0], first)
})

test('whispers can group by author despite negative IDs', () => {
  assert.equal(groupMessages([message(1), { ...message(-1), type: 'whisper' }]).length, 1)
})

test('timestamps use local h:mm AM/PM formatting', () => {
  assert.equal(messageTime(new Date(2026, 0, 1, 13, 5).getTime()), '1:05 PM')
  assert.equal(messageTime(new Date(2026, 0, 1, 0, 5).getTime()), '12:05 AM')
})

test('initial welcome suppresses history and account joins', () => {
  const changes = chatChanges(snapshot({ self: null, users: new Map(), connected: false }), snapshot({ chat: [message(1, 'u:2', '@Alice')] }))
  assert.equal(changes.welcome, true)
  assert.deepEqual(changes.messages, [])
  assert.deepEqual(changes.joined, [])
})

test('reconnect welcome suppresses previously unseen history and changed users', () => {
  const before = snapshot({ chat: [message(1)] })
  const after = snapshot({ self: { ...self }, chat: [message(1), message(2, 'u:2', '@Alice')], users: new Map([['u:3', user('u:3')]]) })
  const changes = chatChanges(before, after)
  assert.equal(changes.welcome, true)
  assert.deepEqual(changes.messages, [])
  assert.deepEqual(changes.joined, [])
  assert.deepEqual(changes.left, [])
})

test('only new live messages from others notify, including whispers', () => {
  const before = snapshot({ chat: [message(1)] })
  const whisper = { ...message(-1), type: 'whisper' }
  const changes = chatChanges(before, { ...before, chat: [...before.chat, message(2, self.key), message(3), whisper] })
  assert.equal(changes.welcome, false)
  assert.deepEqual(changes.messages.map((m) => m.id), [3, -1])
})

test('editing, deleting and updating self never replay message sounds', () => {
  const before = snapshot({ chat: [message(1)] })
  assert.deepEqual(chatChanges(before, { ...before, chat: [{ ...before.chat[0], body: '@Alice', edited: true }] }).messages, [])
  assert.deepEqual(chatChanges(before, { ...before, chat: [{ ...before.chat[0], body: '', deleted: true }] }).messages, [])
  assert.deepEqual(chatChanges(before, { ...before, chat: [] }).messages, [])
  const updated = chatChanges(before, { ...before, self: { ...self, nickname: 'Alicia' } })
  assert.equal(updated.welcome, false)
  assert.deepEqual(updated.messages, [])
})

test('account join/leave diffs exclude anonymous users and profile updates', () => {
  const bob = user('u:2', 'Bob')
  const anonymous = user('a:1234', 'Guest')
  const before = snapshot()
  const after = { ...before, users: new Map([...before.users, [bob.key, bob], [anonymous.key, anonymous]]) }
  assert.deepEqual(chatChanges(before, after).joined, [bob])
  assert.deepEqual(chatChanges(after, before).left, [bob])
  assert.deepEqual(chatChanges(after, { ...after, users: new Map([...after.users, [bob.key, { ...bob, nickname: 'Robert' }]]) }).joined, [])
})

test('disconnected snapshots produce no live messages or temporary lines', () => {
  const before = snapshot()
  const changes = chatChanges(before, { ...before, connected: false, chat: [message(1)], users: new Map([['u:2', user('u:2')]]) })
  assert.deepEqual(changes.messages, [])
  assert.deepEqual(changes.joined, [])
  assert.deepEqual(changes.left, [])
})
