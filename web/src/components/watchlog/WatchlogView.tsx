import { useEffect, useMemo, useState } from 'preact/hooks'
import { watchlog, type Me, type WatchAnime, type WatchPerson, type WatchRange, type Watchlog } from '../../api'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Avatar } from '../ui/Avatar'
import { EmptyState } from '../ui/EmptyState'
import { Notice } from '../ui/Notice'
import { Spinner } from '../ui/Spinner'
import { AnimeSearch } from './AnimeSearch'
import { EpisodesDialog } from './EpisodesDialog'
import { withRange } from './episodes'
import { WatchTable } from './WatchTable'
import styles from './WatchlogView.module.css'

const PEOPLE_KEY = 'cozycast-watchlog-people'

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

// The watchlog itself: the shows, who has watched how far, and adding one.
// In a room (present is given) it starts out comparing the people who are
// there, with the show that plays first; and asks again when version changes.
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
  const [filter, setFilter] = useState('')
  const [removing, setRemoving] = useState<WatchAnime | null>(null)
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

  function choose(username: string) {
    const current = selected.map((p) => p.username)
    const next = current.includes(username) ? current.filter((u) => u !== username) : [...current, username]
    const all = next.length === people.length ? null : next
    setChosen(all)
    keep(all)
  }

  async function add(anime: WatchAnime) {
    setError('')
    try {
      const added = await watchlog.add(anime.id)
      setData((d) => d && {
        ...d,
        anime: [...d.anime.filter((a) => a.id !== added.id), added].sort((a, b) => a.title.localeCompare(b.title)),
      })
    } catch (e) {
      fail(e)
    }
  }

  // Shown at once; asked again if the server says no.
  async function setWatched(animeId: number, username: string, from: number, to: number, watched: boolean) {
    setData((d) => d && {
      ...d,
      progress: [...d.progress.filter((p) => p.animeId !== animeId || p.username !== username),
        { animeId, username, watched: withRange(d.progress.find((p) => p.animeId === animeId && p.username === username)?.watched ?? [], from, to, watched) }],
    })
    setError('')
    try {
      await watchlog.setWatched(animeId, username, from, to, watched)
    } catch (e) {
      fail(e)
      watchlog.get().then(setData).catch(() => {})
    }
  }

  async function remove(anime: WatchAnime) {
    setRemoving(null)
    setError('')
    try {
      await watchlog.remove(anime.id)
      setData((d) => d && { ...d, anime: d.anime.filter((a) => a.id !== anime.id), progress: d.progress.filter((p) => p.animeId !== anime.id) })
    } catch (e) {
      fail(e)
    }
  }

  const detailAnime = detail && data?.anime.find((a) => a.id === detail.animeId)
  const detailPerson = detail && people.find((p) => p.username === detail.username)
  const words = filter.trim().toLowerCase()
  const shown = (data?.anime ?? []).filter((a) => !words || `${a.title} ${a.titleEnglish}`.toLowerCase().includes(words))
  return <>
    <div class={styles.tools}>
      <AnimeSearch onPick={(anime) => { void add(anime) }} placeholder="Add a show: search AniList…" />
      <div class={styles.people} role="group" aria-label="People to compare">
        {people.map((p) => (
          <button key={p.username} type="button" class={styles.chip} aria-pressed={selected.includes(p)} onClick={() => choose(p.username)}>
            <Avatar src={p.avatarUrl} size={20} />
            <span style={{ color: p.nameColor }}>{p.username === user.username ? 'Me' : p.nickname}</span>
          </button>
        ))}
        {chosen && <button type="button" class={styles.everyone} onClick={() => { setChosen(null); keep(null) }}>Everyone</button>}
      </div>
    </div>
    {error && <Notice tone="error">{error}</Notice>}
    {!data && !error && <Spinner label="Loading the watchlog…" />}
    {data && data.anime.length === 0 && (
      <EmptyState icon="monitor" title="No shows yet">Search AniList above to add the first one.</EmptyState>
    )}
    {data && data.anime.length > 0 && <>
      {data.anime.length > 8 && (
        <input class={styles.filter} type="search" placeholder="Filter shows…" aria-label="Filter shows" value={filter}
          onInput={(e) => setFilter(e.currentTarget.value)} />
      )}
      <WatchTable anime={shown} people={selected} self={user.username} admin={user.admin} playing={playing}
        watchedOf={watchedOf} onOpen={(anime, person) => setDetail({ animeId: anime.id, username: person.username })}
        onRemove={setRemoving} />
    </>}
    {detailAnime && detailPerson && (
      <EpisodesDialog anime={detailAnime} person={detailPerson} self={detailPerson.username === user.username}
        watched={watchedOf(detailAnime.id, detailPerson.username)} editable={user.admin || detailPerson.username === user.username}
        onSet={(from, to, watched) => { void setWatched(detailAnime.id, detailPerson.username, from, to, watched) }}
        onClose={() => setDetail(null)} />
    )}
    {removing && (
      <Modal title="Remove show" size="sm" onClose={() => setRemoving(null)} footer={<>
        <Button variant="ghost" onClick={() => setRemoving(null)}>Cancel</Button>
        <Button variant="danger" onClick={() => { void remove(removing) }}>Remove</Button>
      </>}>
        <p class={styles.confirm}>Remove <strong>{removing.title}</strong> and everyone’s progress in it from the watchlog?</p>
      </Modal>
    )}
  </>
}
