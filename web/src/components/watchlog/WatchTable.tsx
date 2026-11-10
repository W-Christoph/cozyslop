import type { WatchAnime, WatchPerson, WatchRange } from '../../api'
import { Button } from '../Button'
import { Avatar } from '../ui/Avatar'
import { Cover } from './AnimeSearch'
import { ProgressCell } from './ProgressCell'
import { finished } from './episodes'
import { together, type Together } from './together'
import styles from './WatchTable.module.css'

// The shows as rows and the people as columns, with what they can watch
// together last. Shows the group can go on with come first.
export function WatchTable({ anime, people, self, admin, playing, watchedOf, ignoredOf, onOpen, onRemove }: {
  anime: WatchAnime[]
  people: WatchPerson[]
  self: string // my username
  admin: boolean
  playing?: number // the anime being watched right now: first, and marked
  watchedOf: (animeId: number, username: string) => readonly WatchRange[]
  ignoredOf: (animeId: number, username: string) => boolean // not following it any more: left out of Together
  onOpen: (anime: WatchAnime, person: WatchPerson) => void // someone's episodes of a show
  onRemove: (anime: WatchAnime) => void
}) {
  const nickname = new Map(people.map((p) => [p.username, p.username === self ? 'You' : p.nickname]))
  // Together is about those in the middle of a show: who has watched some
  // of it, and has not finished it or stopped following it. With nobody in
  // the middle of it, it is the first episode of those who have yet to
  // start; done once everyone has finished.
  const rows = anime.map((a) => {
    const open = people.map((p) => p.username).filter((u) => !finished(watchedOf(a.id, u), a.episodes))
    const following = open.filter((u) => watchedOf(a.id, u).length > 0 && !ignoredOf(a.id, u))
    return { anime: a, together: together(a, following.length ? following : open, (u) => watchedOf(a.id, u)) }
  })
  const going = (t: Together) => t.started && t.next !== null
  rows.sort((a, b) => Number(b.anime.id === playing) - Number(a.anime.id === playing) || Number(going(b.together)) - Number(going(a.together)) || a.anime.title.localeCompare(b.anime.title))
  if (anime.length === 0) return <p class={styles.nothing}>No show matches.</p>
  return (
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
            <th scope="col" class={styles.person}>Together</th>
            {admin && <th scope="col"><span class={styles.hidden}>Remove</span></th>}
          </tr>
        </thead>
        <tbody>
          {rows.map(({ anime: a, together: t }) => (
            <tr key={a.id} class={a.id === playing ? styles.playing : undefined}>
              <th scope="row" class={styles.show}>
                <a class={styles.showLink} href={`https://anilist.co/anime/${a.id}`} target="_blank" rel="noreferrer noopener">
                  <Cover anime={a} size={44} />
                  <span class={styles.titles}>
                    <span class={styles.title}>{a.title}</span>
                    {a.id === playing && <span class={styles.now}>Watching now</span>}
                    {a.titleEnglish && a.titleEnglish !== a.title && <span class={styles.english}>{a.titleEnglish}</span>}
                  </span>
                </a>
              </th>
              {people.map((p) => (
                <td key={p.username} class={styles.cell}>
                  <ProgressCell watched={watchedOf(a.id, p.username)} episodes={a.episodes} ignored={ignoredOf(a.id, p.username)}
                    label={`${nickname.get(p.username)} in ${a.title}`} onOpen={() => onOpen(a, p)} />
                </td>
              ))}
              <td class={styles.cell}><TogetherText together={t} nickname={(u) => nickname.get(u) ?? u} /></td>
              {admin && (
                <td class={styles.cell}>
                  <Button variant="danger-ghost" size="sm" icon="trash" aria-label={`Remove ${a.title}`} title="Remove" onClick={() => onRemove(a)} />
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
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
