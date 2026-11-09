import { useEffect, useMemo, useState } from 'preact/hooks'
import { watchlog, type Me, type WatchAnime, type WatchPerson, type Watchlog } from '../api'
import { me } from '../app/state'
import { Button, ButtonLink } from '../components/Button'
import { Modal } from '../components/Modal'
import { PageLayout } from '../components/PageLayout'
import { AnimeSearch } from '../components/watchlog/AnimeSearch'
import { WatchTable } from '../components/watchlog/WatchTable'
import { Avatar } from '../components/ui/Avatar'
import { EmptyState } from '../components/ui/EmptyState'
import { Notice } from '../components/ui/Notice'
import { Spinner } from '../components/ui/Spinner'
import styles from './WatchlogPage.module.css'

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

const key = (animeId: number, username: string) => `${animeId}:${username}`

export function WatchlogPage() {
  const user = me.value
  return (
    <PageLayout title="Watchlog" subtitle="Who has watched how far, and what you can watch together.">
      {user ? <WatchlogView user={user} /> : <>
        <Notice>Log in to see the watchlog.</Notice>
        <div class={styles.login}><ButtonLink variant="primary" href="/login">Log in</ButtonLink></div>
      </>}
    </PageLayout>
  )
}

function WatchlogView({ user }: { user: Me }) {
  const [data, setData] = useState<Watchlog | null>(null)
  const [error, setError] = useState('')
  const [chosen, setChosen] = useState(loadPeople)
  const [filter, setFilter] = useState('')
  const [removing, setRemoving] = useState<WatchAnime | null>(null)
  const fail = (e: unknown) => setError(e instanceof Error ? e.message : 'Something went wrong.')
  useEffect(() => {
    let current = true
    watchlog.get().then((d) => { if (current) setData(d) }).catch((e) => { if (current) fail(e) })
    return () => { current = false }
  }, [user.username])

  // Everyone with progress, and me; me first.
  const people = useMemo(() => {
    const others = (data?.people ?? []).filter((p) => p.username !== user.username)
    const self: WatchPerson = { username: user.username, nickname: user.nickname, nameColor: user.nameColor, avatarUrl: user.avatarUrl }
    return [self, ...others]
  }, [data, user])
  const selected = people.filter((p) => !chosen || chosen.includes(p.username))
  const progress = useMemo(() => new Map(data?.progress.map((p) => [key(p.animeId, p.username), p.episode])), [data])

  function choose(username: string) {
    const current = selected.map((p) => p.username)
    const next = current.includes(username) ? current.filter((u) => u !== username) : [...current, username]
    const all = next.length === people.length ? null : next
    setChosen(all)
    savePeople(all)
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

  // Shown at once; put back if the server says no.
  async function setProgress(animeId: number, username: string, episode: number) {
    const before = data
    const others = (data?.progress ?? []).filter((p) => p.animeId !== animeId || p.username !== username)
    const changed = episode > 0 ? [{ username, animeId, episode, updatedAt: Date.now() / 1000, source: 'manual' as const }] : []
    setData((d) => d && { ...d, progress: [...others, ...changed] })
    setError('')
    try {
      await watchlog.setProgress(animeId, username, episode)
    } catch (e) {
      setData(before)
      fail(e)
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
        {chosen && <button type="button" class={styles.everyone} onClick={() => { setChosen(null); savePeople(null) }}>Everyone</button>}
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
      <WatchTable anime={shown} people={selected} self={user.username} admin={user.admin}
        episodeOf={(animeId, username) => progress.get(key(animeId, username)) ?? 0}
        onProgress={(animeId, username, episode) => { void setProgress(animeId, username, episode) }}
        onRemove={setRemoving} />
    </>}
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
