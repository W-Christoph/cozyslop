import type { WatchAnime, WatchPerson, WatchProgress } from '../../api'
import { Avatar } from '../ui/Avatar'
import { Cover } from './AnimeSearch'
import { countEpisodes, finished } from './episodes'
import { watchNext } from './together'
import styles from './WatchTonight.module.css'

interface Option {
  anime: WatchAnime
  group: WatchPerson[] // who is following it
  missing: WatchPerson[] // of them, who is not here and has not seen the next episode
  next: number
  behind: { username: string; by: number }[]
  newFor: number // people here who have not started it
  recent: number
}

// What the people here can watch on: the shows some of them are in the
// middle of. A show's group is everyone who has watched some of it, has not
// finished it and has not stopped following it. The people here go on at the first episode one
// of them has not seen; it waits for those who are not here only if they
// have not seen that episode either.
export function WatchTonight({ anime, here, people, progress, ignoredOf, onOpen }: {
  anime: WatchAnime[]
  here: WatchPerson[]
  people: WatchPerson[] // everyone, for those who are not here
  progress: WatchProgress[]
  ignoredOf: (animeId: number, username: string) => boolean
  onOpen: (anime: WatchAnime, person: WatchPerson) => void // someone's episodes: where to say that they stopped following
}) {
  const present = new Set(here.map((p) => p.username))
  const person = new Map(people.map((p) => [p.username, p]))
  const options: Option[] = []
  for (const a of anime) {
    const rows = progress.filter((p) => p.animeId === a.id && countEpisodes(p.watched) > 0 && !finished(p.watched, a.episodes)
      && !ignoredOf(a.id, p.username) && person.has(p.username))
    const watched = new Map(rows.map((p) => [p.username, p.watched]))
    const started = new Set(progress.filter((p) => p.animeId === a.id && countEpisodes(p.watched) > 0).map((p) => p.username))
    const group = rows.map((p) => person.get(p.username)!)
    const t = watchNext(a, group.map((p) => p.username), present, (u) => watched.get(u) ?? [])
    if (!t) continue // nobody here is in the middle of it
    options.push({
      anime: a, group, next: t.next, behind: t.behind,
      missing: group.filter((p) => t.waiting.includes(p.username)),
      newFor: here.filter((p) => !started.has(p.username) && !ignoredOf(a.id, p.username)).length,
      recent: Math.max(...rows.map((p) => p.watchedAt)),
    })
  }
  const rank = (o: Option) => (o.missing.length ? 2 : o.behind.length ? 1 : 0)
  options.sort((a, b) => rank(a) - rank(b) || b.recent - a.recent || a.anime.title.localeCompare(b.anime.title))
  const name = (username: string) => person.get(username)?.nickname ?? username
  return (
    <section class={styles.tonight} aria-label="Watch next">
      <h2 class={styles.heading}>Watch next</h2>
      <p class={styles.about}>Shows the people selected are in the middle of. A show waits for someone who is not among them only if they have not seen the next episode.</p>
      {options.length === 0 ? <p class={styles.nothing}>Nothing in progress for the people selected.</p> : (
        <ul class={styles.list}>
          {options.map((o) => (
            <li key={o.anime.id} class={`${styles.option} ${o.missing.length ? styles.waiting : ''}`}>
              <Cover anime={o.anime} size={52} />
              <div class={styles.text}>
                <strong class={styles.title} title={o.anime.title}>{o.anime.title}</strong>
                <span class={styles.group}>
                  {o.group.map((p) => (
                    <span key={p.username} class={present.has(p.username) ? undefined : styles.away} title={`${p.nickname}${present.has(p.username) ? '' : ' (not here)'}`}>
                      <Avatar src={p.avatarUrl} size={20} />
                    </span>
                  ))}
                </span>
                {o.missing.length > 0 ? (
                  <span class={styles.status}>
                    Waiting for {o.missing.map((p, i) => (
                      <span key={p.username}>
                        {i > 0 && ', '}
                        <button type="button" class={styles.who} title={`${p.nickname}’s episodes: stop waiting for them there`} onClick={() => onOpen(o.anime, p)}>{p.nickname}</button>
                      </span>
                    ))}
                  </span>
                ) : o.behind.length > 0 ? (
                  <span class={`${styles.status} ${styles.catchup}`} title={o.behind.map((b) => `${name(b.username)}: ${b.by} behind`).join('\n')}>
                    {o.behind.length === 1 ? `${name(o.behind[0].username)} is ${o.behind[0].by} behind` : `${o.behind.length} people are behind`}
                  </span>
                ) : (
                  <span class={`${styles.status} ${styles.ready}`}>Ready to watch{o.newFor > 0 && <span class={styles.extra}> · new for {o.newFor} more</span>}</span>
                )}
              </div>
              <span class={styles.next}><small>Next</small>Ep {o.next}</span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
