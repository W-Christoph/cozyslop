import { useState } from 'preact/hooks'
import type { WatchAnime, WatchMode, WatchPerson, WatchRange } from '../../api'
import { Avatar } from '../ui/Avatar'
import { Cover } from './AnimeSearch'
import { ProgressCell } from './ProgressCell'
import { finished } from './episodes'
import { together, type Together } from './together'
import styles from './WatchTable.module.css'

type Status = 'all' | 'going' | 'new' | 'done'
const statuses: { id: Status; label: string }[] = [
  { id: 'all', label: 'All' }, { id: 'going', label: 'In progress' }, { id: 'new', label: 'Not started' }, { id: 'done', label: 'Finished' },
]

// The shows as rows and the people as columns, with what they can watch
// together last. Shows the group can go on with come first. A number opens
// that person's episodes, a show's name the show.
export function WatchTable({ anime, people, self, playing, watchedOf, modeOf, onOpen, onShow }: {
  anime: WatchAnime[]
  people: WatchPerson[]
  self: string // my username
  playing?: number // the anime being watched right now: first, and marked
  watchedOf: (animeId: number, username: string) => readonly WatchRange[]
  modeOf: (animeId: number, username: string) => WatchMode // who does not follow it is left out of Together
  onOpen: (anime: WatchAnime, person: WatchPerson) => void // someone's episodes of a show
  onShow: (anime: WatchAnime) => void
}) {
  const [status, setStatus] = useState<Status>('all')
  const [filter, setFilter] = useState('')
  const nickname = new Map(people.map((p) => [p.username, p.username === self ? 'You' : p.nickname]))
  // Together is about those in the middle of a show: who has watched some
  // of it, has not finished it, and follows it (not casually). With nobody in
  // the middle of it, it is the first episode of those who have yet to
  // start; done once all who started it have finished or dropped it.
  const rows = anime.map((a) => {
    const open = people.map((p) => p.username).filter((u) => !finished(watchedOf(a.id, u), a.episodes))
    const following = open.filter((u) => watchedOf(a.id, u).length > 0 && modeOf(a.id, u) === 'following')
    const started = people.some((p) => watchedOf(a.id, p.username).length > 0)
    return {
      anime: a, going: following.length > 0, started,
      // Once all who started it have finished or dropped it, it is done.
      together: together(a, following.length ? following : started ? [] : open, (u) => watchedOf(a.id, u)),
    }
  })
  rows.sort((a, b) => Number(b.anime.id === playing) - Number(a.anime.id === playing) || Number(b.going) - Number(a.going) || a.anime.title.localeCompare(b.anime.title))
  // In progress while someone is in the middle of it; finished once all who
  // started it have finished or dropped it.
  const statusOf = (r: typeof rows[number]): Status => (r.going ? 'going' : r.started ? 'done' : 'new')
  const words = filter.trim().toLowerCase()
  const shown = rows.filter((r) => (status === 'all' || statusOf(r) === status)
    && (!words || `${r.anime.title} ${r.anime.titleEnglish}`.toLowerCase().includes(words)))
  return <>
    <div class={styles.tools}>
      <div class={styles.status} role="group" aria-label="Shows to list">
        {statuses.map((s) => (
          <button key={s.id} type="button" aria-pressed={status === s.id} onClick={() => setStatus(s.id)}>
            {s.label}<span class={styles.count}>{s.id === 'all' ? rows.length : rows.filter((r) => statusOf(r) === s.id).length}</span>
          </button>
        ))}
      </div>
      {anime.length > 8 && (
        <input class={styles.filter} type="search" placeholder="Filter shows…" aria-label="Filter shows" value={filter}
          onInput={(e) => setFilter(e.currentTarget.value)} onKeyDown={(e) => e.stopPropagation()} />
      )}
    </div>
    <p class={styles.hint}>Click someone’s progress to tick episodes. Click a show for details.</p>
    {shown.length === 0 ? <p class={styles.nothing}>No show matches.</p> : (
    <div class={styles.scroll}>
      <table class={styles.table}>
        <thead>
          <tr>
            <th class={styles.show} scope="col">Show</th>
            {people.map((p) => (
              <th key={p.username} scope="col" class={styles.person}>
                <span class={styles.personName}><Avatar src={p.avatarUrl} size={20} />{nickname.get(p.username)}</span>
              </th>
            ))}
            <th scope="col" class={styles.person}>Next together</th>
          </tr>
        </thead>
        <tbody>
          {shown.map(({ anime: a, together: t }) => (
            <tr key={a.id} class={a.id === playing ? styles.playing : undefined}>
              <th scope="row" class={styles.show}>
                <button type="button" class={styles.showLink} title="Show details and manage" onClick={() => onShow(a)}>
                  <Cover anime={a} size={44} />
                  <span class={styles.titles}>
                    <span class={styles.title}>{a.title}</span>
                    {a.id === playing && <span class={styles.now}>Watching now</span>}
                    {a.titleEnglish && a.titleEnglish !== a.title && <span class={styles.english}>{a.titleEnglish}</span>}
                  </span>
                </button>
              </th>
              {people.map((p) => (
                <td key={p.username} class={styles.cell}>
                  <ProgressCell watched={watchedOf(a.id, p.username)} episodes={a.episodes} mode={modeOf(a.id, p.username)}
                    label={`${nickname.get(p.username)} in ${a.title}`} onOpen={() => onOpen(a, p)} />
                </td>
              ))}
              <td class={styles.cell}><TogetherText together={t} nickname={(u) => nickname.get(u) ?? u} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
    )}
  </>
}

function TogetherText({ together: t, nickname }: { together: Together; nickname: (username: string) => string }) {
  if (t.next === null) return <span class={styles.done}>Done</span>
  const behind = t.behind.length === 0 ? '' : t.behind.length === 1
    ? `${nickname(t.behind[0].username)} ${t.behind[0].by} behind`
    : `${t.behind.length} behind`
  return (
    <span class={styles.together} title={t.behind.map((b) => `${nickname(b.username)}: ${b.by} behind`).join('\n') || undefined}>
      <strong>Ep {t.next}</strong>
      {t.started && behind && <span class={styles.behind}>{behind}</span>}
    </span>
  )
}
