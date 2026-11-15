import { useEffect, useMemo, useState } from 'preact/hooks'
import { watchlog, type Me, type WatchAnime, type WatchPerson, type WatchRange, type Watchlog } from '../../api'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Avatar } from '../ui/Avatar'
import { Icon } from '../ui/Icon'
import { EmptyState } from '../ui/EmptyState'
import { Notice } from '../ui/Notice'
import { Spinner } from '../ui/Spinner'
import { AnimeSearch } from './AnimeSearch'
import { EpisodesDialog } from './EpisodesDialog'
import { hasEpisode, withRange } from './episodes'
import { ShowDialog } from './ShowDialog'
import { WatchNext } from './WatchNext'
import { WatchTable } from './WatchTable'
import styles from './WatchlogView.module.css'

const PEOPLE_KEY = 'cozycast-watchlog-people'
const TAB_KEY = 'cozycast-watchlog-tab'

// The watchlog's two views: what to watch now, and who has watched what.
type Tab = 'next' | 'progress'
const tabs: { id: Tab; icon: 'play' | 'users'; label: string; about: string }[] = [
  { id: 'next', icon: 'play', label: 'Watch next', about: 'What can we watch now?' },
  { id: 'progress', icon: 'users', label: 'Progress', about: 'Who has watched what' },
]

function loadTab(): Tab {
  try {
    return localStorage.getItem(TAB_KEY) === 'progress' ? 'progress' : 'next'
  } catch {
    return 'next'
  }
}

// Who the table compares, remembered in this browser; null: everyone.
function loadPeople(): string[] | null {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(PEOPLE_KEY) ?? 'null')
    return Array.isArray(stored) && stored.every((s) => typeof s === 'string') ? stored : null
  } catch {
    return null
  }
}

function savePeople(people: string[] | null) {
  try {
    if (people) localStorage.setItem(PEOPLE_KEY, JSON.stringify(people))
    else localStorage.removeItem(PEOPLE_KEY)
  } catch {
    // Not remembered, then.
  }
}

const none: readonly WatchRange[] = []
const key = (animeId: number, username: string) => `${animeId}:${username}`

// The watchlog itself, as two tabs: what the people selected can watch now
// (WatchNext), and who has watched how far (WatchTable). A show opens with
// everything to do about it (ShowDialog). In a room (present is given) it
// starts out on what to watch, with the people who are there and the show
// that plays first; and asks again when version changes.
export function WatchlogView({ user, present, playing, version }: {
  user: Me
  present?: WatchPerson[] // the accounts in the room
  playing?: number // the anime the room is watching
  version?: unknown
}) {
  const [data, setData] = useState<Watchlog | null>(null)
  const [error, setError] = useState('')
  // The room's choice is for this visit; the page's is remembered.
  const [chosen, setChosen] = useState(() => present ? [user.username, ...present.map((p) => p.username)] : loadPeople())
  const keep = (people: string[] | null) => { if (!present) savePeople(people) }
  const [tab, setTab] = useState<Tab>(() => (present ? 'next' : loadTab()))
  const [adding, setAdding] = useState(false)
  const [show, setShow] = useState<number | null>(null) // the show that is open
  // Bumped to sort Watch next's shows into their sections again (WatchNext).
  const [layout, setLayout] = useState(0)
  const relayout = () => setLayout((n) => n + 1)
  const [refreshing, setRefreshing] = useState(false)
  const [removing, setRemoving] = useState<WatchAnime | null>(null)
  // What was just ticked from a card, to take back.
  const [marked, setMarked] = useState<{ anime: WatchAnime; episode: number; usernames: string[] } | null>(null)
  // Whose episodes of which show are open.
  const [detail, setDetail] = useState<{ animeId: number; username: string } | null>(null)
  const fail = (e: unknown) => setError(e instanceof Error ? e.message : 'Something went wrong.')
  useEffect(() => {
    let current = true
    watchlog.get().then((d) => { if (current) setData(d) }).catch((e) => { if (current) fail(e) })
    return () => { current = false }
  }, [user.username, version])

  // Everyone with progress or in the room, and me; me first.
  const people = useMemo(() => {
    const self: WatchPerson = { username: user.username, nickname: user.nickname, nameColor: user.nameColor, avatarUrl: user.avatarUrl }
    const others = new Map([...(data?.people ?? []), ...(present ?? [])].map((p) => [p.username, p]))
    others.delete(self.username)
    return [self, ...others.values()]
  }, [data, user, present])
  const selected = people.filter((p) => !chosen || chosen.includes(p.username))
  const progress = useMemo(() => new Map(data?.progress.map((p) => [key(p.animeId, p.username), p.watched])), [data])
  const watchedOf = (animeId: number, username: string) => progress.get(key(animeId, username)) ?? none
  const ignored = useMemo(() => new Set(data?.ignored.map((i) => key(i.animeId, i.username))), [data])
  const ignoredOf = (animeId: number, username: string) => ignored.has(key(animeId, username))
  const mayChange = (username: string) => user.admin || username === user.username

  function choose(username: string) {
    const current = selected.map((p) => p.username)
    const next = current.includes(username) ? current.filter((u) => u !== username) : [...current, username]
    const all = next.length === people.length ? null : next
    setChosen(all)
    keep(all)
  }

  function open(next: Tab) {
    setTab(next)
    setMarked(null)
    if (present) return
    try {
      localStorage.setItem(TAB_KEY, next)
    } catch {
      // Not remembered, then.
    }
  }

  // Adds a show, or brings one up to date from AniList; and opens it.
  async function add(id: number) {
    setError('')
    setRefreshing(true)
    try {
      const added = await watchlog.add(id)
      setData((d) => d && {
        ...d,
        anime: [...d.anime.filter((a) => a.id !== added.id), added].sort((a, b) => a.title.localeCompare(b.title)),
      })
      setShow(added.id)
    } catch (e) {
      fail(e)
    } finally {
      setRefreshing(false)
    }
  }

  // Ticks an episode for those who have not seen it, to be taken back.
  function mark(anime: WatchAnime, episode: number, usernames: string[]) {
    const fresh = usernames.filter((u) => !hasEpisode(watchedOf(anime.id, u), episode))
    if (fresh.length === 0) return
    setMarked({ anime, episode, usernames: fresh })
    for (const u of fresh) void setWatched(anime.id, u, episode, episode, true)
  }

  // The offer to take it back goes by itself.
  useEffect(() => {
    if (!marked) return
    const timer = window.setTimeout(() => setMarked(null), 10000)
    return () => window.clearTimeout(timer)
  }, [marked])

  function unmark() {
    if (!marked) return
    for (const u of marked.usernames) void setWatched(marked.anime.id, u, marked.episode, marked.episode, false)
    setMarked(null)
  }

  // Shown at once; asked again if the server says no.
  async function setWatched(animeId: number, username: string, from: number, to: number, watched: boolean) {
    setData((d) => d && {
      ...d,
      progress: [...d.progress.filter((p) => p.animeId !== animeId || p.username !== username),
        { animeId, username, watchedAt: Date.now() / 1000, watched: withRange(d.progress.find((p) => p.animeId === animeId && p.username === username)?.watched ?? [], from, to, watched) }],
    })
    setError('')
    try {
      await watchlog.setWatched(animeId, username, from, to, watched)
    } catch (e) {
      fail(e)
      watchlog.get().then(setData).catch(() => {})
    }
  }

  async function setIgnored(animeId: number, username: string, ignored: boolean) {
    relayout()
    setData((d) => d && {
      ...d,
      ignored: [...d.ignored.filter((i) => i.animeId !== animeId || i.username !== username), ...(ignored ? [{ animeId, username }] : [])],
    })
    setError('')
    try {
      await watchlog.setIgnored(animeId, username, ignored)
    } catch (e) {
      fail(e)
      watchlog.get().then(setData).catch(() => {})
    }
  }

  async function remove(anime: WatchAnime) {
    setRemoving(null)
    setShow(null)
    setMarked(null)
    setError('')
    try {
      await watchlog.remove(anime.id)
      setData((d) => d && {
        ...d, anime: d.anime.filter((a) => a.id !== anime.id), progress: d.progress.filter((p) => p.animeId !== anime.id),
        ignored: d.ignored.filter((i) => i.animeId !== anime.id),
      })
    } catch (e) {
      fail(e)
    }
  }

  const detailAnime = detail && data?.anime.find((a) => a.id === detail.animeId)
  const detailPerson = detail && people.find((p) => p.username === detail.username)
  const shown = show !== null && data?.anime.find((a) => a.id === show)
  const name = (username: string) => (username === user.username ? 'you' : people.find((p) => p.username === username)?.nickname ?? username)
  const episodesOf = (anime: WatchAnime, person: WatchPerson) => setDetail({ animeId: anime.id, username: person.username })
  return <>
    <div class={styles.top}>
      <div class={styles.tabs} role="tablist" aria-label="Watchlog">
        {tabs.map((t) => (
          <button key={t.id} type="button" role="tab" id={`watchlog-tab-${t.id}`} class={styles.tab} aria-selected={tab === t.id} aria-controls="watchlog-panel"
            tabIndex={tab === t.id ? 0 : -1} onClick={() => open(t.id)} onKeyDown={(e) => {
              // Arrow keys move between the tabs (the ARIA tabs pattern).
              if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
              e.preventDefault()
              const to = tabs[(tabs.findIndex((x) => x.id === t.id) + (e.key === 'ArrowRight' ? 1 : tabs.length - 1)) % tabs.length].id
              open(to)
              document.getElementById(`watchlog-tab-${to}`)?.focus()
            }}>
            <Icon name={t.icon} size={18} />
            <span class={styles.tabText}><strong>{t.label}</strong><span>{t.about}</span></span>
          </button>
        ))}
      </div>
      <Button variant="primary" icon="plus" onClick={() => setAdding(true)}>Add show</Button>
    </div>
    {error && <Notice tone="error" class={styles.notice}>{error}</Notice>}
    {marked && (
      <div key={`${marked.anime.id}:${marked.episode}`} class={styles.undo} role="status">
        <Icon name="check" size={16} />
        <span>Marked episode {marked.episode} of <strong>{marked.anime.title}</strong> as watched for {marked.usernames.map(name).join(', ')}.</span>
        <Button size="sm" onClick={unmark}>Undo</Button>
        <Button size="sm" variant="ghost" icon="x" aria-label="Dismiss" onClick={() => setMarked(null)} />
      </div>
    )}
    {!data && !error && <Spinner label="Loading the watchlog…" />}
    {data && data.anime.length === 0 && (
      <EmptyState icon="monitor" title="No shows yet">
        <p class={styles.confirm}>Add the shows you watch together. The watchlog then keeps track of who has seen which episode.</p>
        <Button variant="primary" icon="plus" class={styles.first} onClick={() => setAdding(true)}>Add the first show</Button>
      </EmptyState>
    )}
    {data && data.anime.length > 0 && (
      <div id="watchlog-panel" class={styles.panel} role="tabpanel" aria-labelledby={`watchlog-tab-${tab}`}>
        <div class={styles.people} role="group" aria-label={tab === 'next' ? 'Who is watching' : 'People to compare'}>
          <span class={styles.peopleLabel}>{tab === 'next' ? 'Who is watching?' : 'Compare'}</span>
          {people.map((p) => (
            <button key={p.username} type="button" class={styles.chip} aria-pressed={selected.includes(p)} onClick={() => choose(p.username)}>
              <Avatar src={p.avatarUrl} size={20} />
              <span style={selected.includes(p) ? { color: p.nameColor } : undefined}>{p.username === user.username ? 'You' : p.nickname}</span>
            </button>
          ))}
          <button type="button" class={styles.everyone} disabled={!chosen} onClick={() => { setChosen(null); keep(null) }}>Everyone</button>
        </div>
        {tab === 'next' ? (
          <WatchNext anime={data.anime} here={selected} people={people} progress={data.progress} self={user.username} admin={user.admin}
            playing={playing} layout={layout} heading={present ? 'h3' : 'h2'} ignoredOf={ignoredOf} onShow={(anime) => setShow(anime.id)} onOpen={episodesOf} onMark={mark} />
        ) : (
          <WatchTable anime={data.anime} people={selected} self={user.username} playing={playing}
            watchedOf={watchedOf} ignoredOf={ignoredOf} onOpen={episodesOf} onShow={(anime) => setShow(anime.id)} />
        )}
      </div>
    )}
    {adding && (
      <Modal title="Add a show" size="md" onClose={() => setAdding(false)}>
        <p class={styles.confirm}>Search AniList and pick the show. It is added for everyone; afterwards you can tick what you have already seen.</p>
        <div class={styles.adding}>
          <AnimeSearch autoFocus placeholder="Type a title, e.g. Frieren…" onPick={(anime) => { setAdding(false); void add(anime.id) }} />
        </div>
      </Modal>
    )}
    {shown && data && (
      <ShowDialog anime={shown} self={user.username} playing={shown.id === playing} busy={refreshing}
        people={people.filter((p) => p.username === user.username || selected.includes(p) || watchedOf(shown.id, p.username).length > 0)}
        watchedOf={(u) => watchedOf(shown.id, u)} ignoredOf={(u) => ignoredOf(shown.id, u)} mayChange={mayChange}
        mayRemove={user.admin || !data.progress.some((p) => p.animeId === shown.id && p.watched.length > 0)}
        onEpisodes={(person) => episodesOf(shown, person)} onSet={(u, episode) => { void setWatched(shown.id, u, episode, episode, true) }}
        onIgnore={(u, value) => { void setIgnored(shown.id, u, value) }} onRefresh={() => { void add(shown.id) }}
        onRemove={() => setRemoving(shown)} onClose={() => { setShow(null); relayout() }} />
    )}
    {detailAnime && detailPerson && (
      <EpisodesDialog anime={detailAnime} person={detailPerson} self={detailPerson.username === user.username}
        watched={watchedOf(detailAnime.id, detailPerson.username)} editable={mayChange(detailPerson.username)}
        ignored={ignoredOf(detailAnime.id, detailPerson.username)} onIgnore={(value) => { void setIgnored(detailAnime.id, detailPerson.username, value) }}
        onSet={(from, to, watched) => { void setWatched(detailAnime.id, detailPerson.username, from, to, watched) }}
        onClose={() => { setDetail(null); relayout() }} />
    )}
    {removing && (
      <Modal title="Remove show" size="sm" onClose={() => setRemoving(null)} footer={<>
        <Button variant="ghost" onClick={() => setRemoving(null)}>Cancel</Button>
        <Button variant="danger" onClick={() => { void remove(removing) }}>Remove</Button>
      </>}>
        <p class={styles.confirm}>Remove <strong>{removing.title}</strong> and everyone’s progress in it from the watchlog? This cannot be undone.</p>
      </Modal>
    )}
  </>
}
