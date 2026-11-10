import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let episodes, together, watchNext
try {
  episodes = await server.ssrLoadModule('/src/components/watchlog/episodes.ts')
  ; ({ together, watchNext } = await server.ssrLoadModule('/src/components/watchlog/together.ts'))
} finally { await server.close() }

test('ticking and unticking episodes keeps the ranges in order and joined', () => {
  const { withRange } = episodes
  assert.deepEqual(withRange([], 5, 5, true), [[5, 5]])
  assert.deepEqual(withRange([[5, 5]], 1, 3, true), [[1, 3], [5, 5]])
  assert.deepEqual(withRange([[1, 3], [5, 5]], 4, 4, true), [[1, 5]])
  assert.deepEqual(withRange([[1, 5]], 3, 3, false), [[1, 2], [4, 5]])
  assert.deepEqual(withRange([[1, 2], [4, 5]], 1, 9999, false), [])
  assert.deepEqual(withRange([[2, 4], [8, 9]], 1, 8, true), [[1, 9]])
})

test('what is watched, where to go on and what was skipped', () => {
  const { hasEpisode, countEpisodes, firstMissing, lastEpisode, skipped, rangeText } = episodes
  const watched = [[1, 1], [5, 6], [9, 9]]
  assert.equal(hasEpisode(watched, 5), true)
  assert.equal(hasEpisode(watched, 4), false)
  assert.equal(countEpisodes(watched), 4)
  assert.equal(firstMissing(watched), 2)
  assert.equal(firstMissing([[5, 5]]), 1)
  assert.equal(firstMissing([]), 1)
  assert.equal(lastEpisode(watched), 9)
  assert.deepEqual(skipped(watched).map(rangeText), ['2–4', '7–8'])
  assert.deepEqual(skipped([[1, 12]]), [])
  assert.equal(episodes.finished([[1, 12]], 12), true)
  assert.equal(episodes.finished([[1, 11]], 12), false)
  assert.equal(episodes.finished([[1, 500]], null), false)
})

test('together goes on at the first episode someone has not watched', () => {
  const anime = { id: 1, title: 'Show', titleEnglish: '', episodes: 12, coverUrl: '' }
  const of = { a: [[1, 5]], b: [[1, 2], [5, 5]], c: [[1, 12]] }
  assert.deepEqual(together(anime, ['a', 'b'], (u) => of[u]), { next: 3, started: true, behind: [{ username: 'b', by: 2 }] })
  assert.equal(together(anime, ['c'], (u) => of[u]).next, null)
  assert.deepEqual(together(anime, ['a', 'd'], (u) => of[u] ?? []), { next: 1, started: true, behind: [{ username: 'd', by: 5 }] })
  assert.deepEqual(together(anime, [], () => []), { next: null, started: false, behind: [] })
})

test('a show waits only for someone away who has not seen the next episode', () => {
  const anime = { id: 1, title: 'Show', titleEnglish: '', episodes: 12, coverUrl: '' }
  const of = { me: [[1, 4]], ahead: [[1, 5]], behind: [[1, 2]], done: [[1, 12]] }
  const next = (group, here) => watchNext(anime, group, new Set(here), (u) => of[u] ?? [])
  // I go on at 5, which the one away has seen: nothing to wait for.
  assert.deepEqual(next(['me', 'ahead'], ['me']), { next: 5, behind: [], waiting: [] })
  // The one away has not seen 5.
  assert.deepEqual(next(['me', 'behind'], ['me']), { next: 5, behind: [], waiting: ['behind'] })
  // Here together: 3 is next, and I would see it again.
  assert.deepEqual(next(['me', 'behind'], ['me', 'behind']), { next: 3, behind: [{ username: 'behind', by: 2 }], waiting: [] })
  assert.equal(next(['me', 'ahead'], ['other']), null)
  assert.equal(next(['done', 'me'], ['done']), null)
})
