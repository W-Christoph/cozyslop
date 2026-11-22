import test from 'node:test'
import assert from 'node:assert/strict'
import { createServer } from 'vite'

const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let episodes, together, watchNext, watchOptions, airing
try {
  episodes = await server.ssrLoadModule('/src/components/watchlog/episodes.ts')
  airing = await server.ssrLoadModule('/src/components/watchlog/airing.ts')
  ; ({ together, watchNext, watchOptions } = await server.ssrLoadModule('/src/components/watchlog/together.ts'))
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
  assert.deepEqual(together(anime, ['a', 'b'], (u) => of[u]), { next: 3, started: true, behind: [{ username: 'b', by: 3 }] })
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

test('what to watch is sorted into together, some of us, waiting and not started', () => {
  const show = (id, title) => ({ id, title, titleEnglish: '', episodes: 12, coverUrl: '', addedAt: id })
  const anime = [show(1, 'Ready'), show(2, 'Catchup'), show(3, 'Waiting'), show(4, 'New'), show(5, 'Finished'), show(6, 'Dropped'), show(7, 'Newer'), show(8, 'Solo')]
  const row = (animeId, username, watched) => ({ animeId, username, watched, watchedAt: animeId })
  const progress = [
    row(1, 'me', [[1, 4]]), row(1, 'bob', [[1, 4]]),
    row(2, 'me', [[1, 4]]), row(2, 'bob', [[1, 2]]),
    row(3, 'me', [[1, 4]]), row(3, 'away', [[1, 2]]),
    row(5, 'me', [[1, 12]]),
    row(6, 'away', [[1, 3]]),
    row(8, 'bob', [[1, 3]]),
  ]
  const ignored = (animeId, username) => (animeId === 6 && username === 'bob' ? 'dropped' : 'following')
  const options = watchOptions(anime, ['me', 'bob'], progress, ignored)
  assert.deepEqual(options.map((o) => [o.anime.title, o.state, o.next]),
    [['Ready', 'together', 5], ['Catchup', 'together', 3], ['Solo', 'some', 4], ['Waiting', 'waiting', 5], ['Newer', 'new', 1], ['Dropped', 'new', 1], ['New', 'new', 1]])
  const by = Object.fromEntries(options.map((o) => [o.anime.title, o]))
  assert.deepEqual([...by.Ready.needs].sort(), ['bob', 'me'])
  assert.deepEqual(by.Catchup.needs, ['bob'])
  assert.deepEqual(by.Waiting.waiting, ['away'])
  assert.deepEqual(by.Waiting.unstarted, ['bob'])
  // Bob dropped it before starting: it would be new for me alone.
  assert.deepEqual(by.Dropped.needs, ['me'])
  assert.deepEqual(by.Dropped.group, ['away'])
  // Alone, what one watches is for everyone here.
  assert.equal(watchOptions(anime, ['bob'], progress, ignored).find((o) => o.anime.title === 'Solo').state, 'together')
  assert.deepEqual(watchOptions(anime, [], progress, ignored), [])
})

test('who watches a show casually is not waited for and holds nobody back', () => {
  const anime = [{ id: 1, title: 'Show', titleEnglish: '', episodes: 12, coverUrl: '', addedAt: 1 }]
  const row = (username, watched) => ({ animeId: 1, username, watched, watchedAt: 1 })
  const progress = [row('me', [[1, 6]]), row('bob', [[1, 6]]), row('cas', [[2, 2], [5, 5]])]
  const mode = (_, username) => (username === 'cas' ? 'casual' : 'following')
  // Away, cas is not waited for, though they have not seen episode 7.
  const [away] = watchOptions(anime, ['me', 'bob'], progress, mode)
  assert.deepEqual([away.state, away.next, away.waiting, away.group], ['together', 7, [], ['me', 'bob']])
  // Here, the next episode is still the others': cas sees it with them.
  const [here] = watchOptions(anime, ['me', 'bob', 'cas'], progress, mode)
  assert.deepEqual([here.state, here.next, here.behind, here.casual], ['together', 7, [], ['cas']])
  assert.deepEqual(here.needs, ['me', 'bob', 'cas'])
  // Following it, cas would hold it up either way.
  assert.equal(watchOptions(anime, ['me', 'bob'], progress, () => 'following')[0].state, 'waiting')
  assert.equal(watchOptions(anime, ['me', 'bob', 'cas'], progress, () => 'following')[0].next, 1)
  // Nobody who follows it is here: it is not theirs to watch on.
  assert.deepEqual(watchOptions(anime, ['cas'], progress, mode), [])
})

test('a show whose next episode has not aired waits for it, soonest first', () => {
  const now = Date.now() / 1000
  const show = (id, nextEpisode, inHours) => ({ id, title: `Show ${id}`, titleEnglish: '', episodes: null, coverUrl: '', addedAt: id,
    nextEpisode, nextAiringAt: nextEpisode === null ? null : now + inHours * 3600 })
  const later = show(1, 8, 48), soon = show(2, 8, 2), out = show(3, 8, -1), upcoming = show(4, 1, 24)
  assert.equal(airing.aired(later, 7), true)
  assert.equal(airing.aired(later, 8), false)
  assert.equal(airing.aired(out, 8), true)
  assert.equal(airing.aired(out, 9), false)
  assert.equal(airing.aired(show(5, null), 99), true)
  assert.equal(airing.airsIn(now + 2 * 86400 + 5 * 3600 + 1), 'in 2d 5h')
  assert.equal(airing.airsIn(now + 3 * 3600 + 20 * 60), 'in 3h 20m')
  assert.equal(airing.airsIn(now + 60 * 12), 'in 12m')
  assert.equal(airing.episodeAddress({ watchUrl: 'https://site/show?ep={ep}' }, 9), 'https://site/show?ep=9')
  assert.equal(airing.episodeAddress({ watchUrl: '' }, 9), '')

  // Everyone has seen 1 to 7.
  const progress = [1, 2, 3].map((animeId) => ({ animeId, username: 'a', watched: [[1, 7]], watchedAt: 1 }))
  const options = watchOptions([later, soon, out, upcoming], ['a'], progress, () => 'following')
  assert.deepEqual(options.map((o) => [o.anime.id, o.state, o.next]), [[3, 'together', 8], [2, 'airing', 8], [4, 'airing', 1], [1, 'airing', 8]])
})
