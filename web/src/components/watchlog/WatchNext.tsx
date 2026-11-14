import { useRef, useState } from 'preact/hooks'
import type { WatchAnime, WatchPerson, WatchProgress } from '../../api'
import { Button } from '../Button'
import { Avatar } from '../ui/Avatar'
import { Icon } from '../ui/Icon'
import { Cover } from './AnimeSearch'
import { finished, firstMissing } from './episodes'
import { watchOptions, watchStates, type WatchOption, type WatchState } from './together'
import styles from './WatchNext.module.css'

const sections: Record<WatchState, { title: string; about: string }> = {
  ready: { title: 'Ready to watch', about: 'You’re all on the same episode.' },
  catchup: { title: 'Someone has to catch up', about: 'Someone is behind. Start at their next episode; the others see it again.' },
  waiting: { title: 'Waiting for someone', about: 'Someone who watches this isn’t here. Watching on would leave them behind.' },
  solo: { title: 'Only one of you watches these', about: 'The others here have not started them.' },
  new: { title: 'Not started yet', about: 'Nobody here has started these.' },
}

// Sections that can get long list a few shows, and the rest on request.
const few: Partial<Record<WatchState, number>> = { solo: 3, new: 6 }

// What the people selected can watch now, sorted into what it takes: ready,
// catching up, waiting for someone who is away, shows only one of them
// watches and shows yet to start (together.ts). A card says who has seen how
// much of the show and where that leaves me; it opens the show, a person on
// it their episodes, and its buttons tick the next episode.
export function WatchNext({ anime, here, people, progress, self, admin, playing, ignoredOf, onShow, onOpen, onMark }: {
  anime: WatchAnime[]
  here: WatchPerson[]
  people: WatchPerson[] // everyone, for those who are not here
  progress: WatchProgress[]
  self: string // my username
  admin: boolean
  playing?: number // the anime being watched right now: first in its section, and marked
  ignoredOf: (animeId: number, username: string) => boolean
  onShow: (anime: WatchAnime) => void
  onOpen: (anime: WatchAnime, person: WatchPerson) => void // someone's episodes
  onMark: (anime: WatchAnime, episode: number, usernames: string[]) => void
}) {
  const [all, setAll] = useState<Partial<Record<WatchState, boolean>>>({}) // sections listed in full
  const person = new Map(people.map((p) => [p.username, p]))
  const present = new Set(here.map((p) => p.username))
  const watched = new Map(progress.map((p) => [`${p.animeId}:${p.username}`, p.watched]))
  const watchedOf = (animeId: number, username: string) => watched.get(`${animeId}:${username}`) ?? []
  // The last episode someone has seen without leaving one out before it.
  const seen = (animeId: number, username: string) => firstMissing(watchedOf(animeId, username)) - 1
  const options = watchOptions(anime, here.map((p) => p.username), progress, ignoredOf)
  // A show stays where it stood when these people were picked: ticking an
  // episode changes what its card says, and moves nothing under the pointer.
  const who = here.map((p) => p.username).join(' ')
  const placed = useRef<{ who: string; at: Map<number, { state: WatchState; index: number }> } | null>(null)
  if (placed.current?.who !== who) placed.current = { who, at: new Map() }
  const at = placed.current.at
  for (const o of options) if (!at.has(o.anime.id)) at.set(o.anime.id, { state: o.state, index: at.size })
  const place = (o: WatchOption) => at.get(o.anime.id)!

  const name = (username: string) => (username === self ? 'You' : person.get(username)?.nickname ?? username)
  const list = (names: string[]) => (names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0])
  // "You and Bob are": the names with their verb.
  const names = (usernames: string[], one: string, many: string) =>
    `${list(usernames.map(name))} ${usernames.length === 1 && usernames[0] !== self ? one : many}`
  if (here.length === 0) return <p class={styles.nothing}>Select who is watching above.</p>
  if (options.length === 0) return <p class={styles.nothing}>Nothing to watch for the people selected: they have finished everything on the list. Add a show.</p>

  // What it takes, in a few words.
  const status = (o: WatchOption) => {
    const watching = o.group.filter((u) => present.has(u))
    if (o.state === 'new') return 'Nobody here has started it'
    if (o.state === 'waiting') return `Waiting for ${list(o.waiting.map(name))}`
    if (o.behind.length > 0) return o.behind.map((b) => `${names([b.username], 'is', 'are')} ${b.by} behind`).join(', ')
    if (watching.length === 1) return watching[0] === self ? 'Only you watch this' : `Only ${name(watching[0])} watches this`
    return watching.length === here.length ? `Everyone here is on ep ${o.next}` : `${names(watching, 'is', 'are')} on ep ${o.next}`
  }

  const buttons = (o: WatchOption) => {
    const others = o.needs.filter((u) => u !== self)
    return <>
      {o.needs.includes(self) && (
        <Button size="sm" icon="check" title={`Mark episode ${o.next} as watched for you`} onClick={() => onMark(o.anime, o.next, [self])}>I watched ep {o.next}</Button>
      )}
      {admin && others.length > 0 && (
        <Button size="sm" icon="check" title={`Mark episode ${o.next} as watched for ${list(o.needs.map(name))}`}
          onClick={() => onMark(o.anime, o.next, o.needs)}>{o.needs.length === 1 ? `${name(o.needs[0])} watched ep ${o.next}` : `All ${o.needs.length} watched ep ${o.next}`}</Button>
      )}
    </>
  }

  // A show nobody here has started: one line.
  const row = (o: WatchOption) => (
    <li key={o.anime.id} class={styles.row}>
      <button type="button" class={styles.rowMain} title="Show details" onClick={() => onShow(o.anime)}>
        <Cover anime={o.anime} size={44} />
        <span class={styles.text}>
          <strong class={styles.title}>{o.anime.title}</strong>
          <span class={styles.detail}>
            {[o.anime.episodes !== null ? `${o.anime.episodes} episodes` : 'Still airing', o.state !== 'new' ? status(o) : o.group.length > 0 ? `${names(o.group, 'watches', 'watch')} it` : '']
              .filter(Boolean).join(' · ')}
          </span>
        </span>
      </button>
      <div class={styles.rowActions}>{buttons(o)}</div>
    </li>
  )

  const card = (o: WatchOption) => {
    const a = o.anime.id
    const behind = new Set(o.behind.map((b) => b.username))
    const done = here.map((p) => p.username).filter((u) => finished(watchedOf(a, u), o.anime.episodes))
    // Everyone the show concerns, told apart by how their chip looks: who
    // watches it here with the episode they have seen up to (amber: behind),
    // who follows it but is away (grey, dashed), who has finished (a tick),
    // and last who has yet to start, as a face alone: nothing to put right.
    const chips = [
      ...o.group.filter((u) => present.has(u)).map((u) => ({ u, kind: behind.has(u) ? 'behind' : 'level', title: `${name(u)}: seen up to episode ${seen(a, u)}${behind.has(u) ? ', behind the others' : ''}` })),
      ...o.group.filter((u) => !present.has(u)).map((u) => ({ u, kind: 'away', title: `${name(u)}: not selected, seen up to episode ${seen(a, u)}` })),
      ...done.map((u) => ({ u, kind: 'done', title: `${name(u)}: finished it` })),
    ].sort((x, y) => Number(y.u === self) - Number(x.u === self))
      .concat(o.unstarted.map((u) => ({ u, kind: 'idle', title: `${name(u)}: not started` })))
    const moved = o.state !== place(o).state
    return (
      <li key={o.anime.id} class={`${styles.option} ${styles[o.state]} ${o.anime.id === playing ? styles.playing : ''}`}>
        <button type="button" class={styles.main} title="Show details" onClick={() => onShow(o.anime)}>
          <Cover anime={o.anime} size={64} />
          <span class={styles.text}>
            <strong class={styles.title}>{o.anime.title}</strong>
            {o.anime.id === playing && <span class={styles.now}>Watching now</span>}
          </span>
          <span class={styles.next}><small>Next</small>Ep {o.next}</span>
        </button>
        <ul class={styles.people} aria-label="Who has seen how much">
          {chips.map(({ u, kind, title }) => {
            const p = person.get(u)
            return (
              <li key={u}>
                <button type="button" class={`${styles.person} ${styles[kind]} ${u === self ? styles.me : ''}`} title={`${title}. Open ${u === self ? 'your' : 'their'} episodes`}
                  aria-label={title} disabled={!p} onClick={() => { if (p) onOpen(o.anime, p) }}>
                  <Avatar src={p?.avatarUrl} size={18} />
                  {kind !== 'idle' && <span class={styles.personName}>{name(u)}</span>}
                  {kind === 'done' ? <Icon name="check" size={13} class={styles.tick} /> : kind !== 'idle' && <span class={styles.at}>ep {seen(a, u)}</span>}
                </button>
              </li>
            )
          })}
        </ul>
        <div class={styles.actions}>
          {buttons(o)}
          {moved && <span class={styles.moved}>Now: {sections[o.state].title.toLowerCase()}</span>}
          <Button size="sm" variant="ghost" class={styles.details} onClick={() => onShow(o.anime)}>Details</Button>
        </div>
      </li>
    )
  }

  return <>
    <ul class={styles.key} aria-label="What the people on a show look like">
      <li><span class={`${styles.person} ${styles.level}`}><Avatar size={18} /><span class={styles.at}>ep 5</span></span>seen up to</li>
      <li><span class={`${styles.person} ${styles.behind}`}><Avatar size={18} /><span class={styles.at}>ep 3</span></span>behind</li>
      <li><span class={`${styles.person} ${styles.done}`}><Avatar size={18} /><Icon name="check" size={13} class={styles.tick} /></span>finished</li>
      <li><span class={`${styles.person} ${styles.idle}`}><Avatar size={18} /></span>not started</li>
      <li><span class={`${styles.person} ${styles.away}`}><Avatar size={18} /><span class={styles.at}>ep 5</span></span>not selected</li>
    </ul>
    {watchStates.map((state) => {
      // The show that plays comes first where it stands.
      const found = options.filter((o) => place(o).state === state)
        .sort((a, b) => Number(b.anime.id === playing) - Number(a.anime.id === playing) || place(a).index - place(b).index)
      if (found.length === 0) return null
      const limit = few[state] ?? found.length
      const shown = all[state] ? found : found.slice(0, limit)
      return (
        <section key={state} class={styles.section} aria-label={sections[state].title}>
          <h3 class={styles.heading}><span class={`${styles.dot} ${styles[state]}`} />{sections[state].title}<span class={styles.count}>{found.length}</span></h3>
          <p class={styles.about}>{sections[state].about}</p>
          {state === 'new' ? <ul class={styles.rows}>{shown.map(row)}</ul> : <ul class={styles.list}>{shown.map(card)}</ul>}
          {found.length > limit && (
            <Button size="sm" class={styles.more} aria-expanded={!!all[state]} onClick={() => setAll({ ...all, [state]: !all[state] })}>
              {all[state] ? 'Show fewer' : `Show all ${found.length}`}
            </Button>
          )}
        </section>
      )
    })}
  </>
}
