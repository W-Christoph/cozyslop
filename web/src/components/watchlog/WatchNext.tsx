import { useRef, useState } from 'preact/hooks'
import type { WatchAnime, WatchMode, WatchPerson, WatchProgress } from '../../api'
import { Button } from '../Button'
import { Avatar } from '../ui/Avatar'
import { Icon, type IconName } from '../ui/Icon'
import { Cover } from './AnimeSearch'
import { countEpisodes, finished, firstMissing } from './episodes'
import { watchOptions, watchStates, type WatchOption, type WatchState } from './together'
import styles from './WatchNext.module.css'

const sections: Record<WatchState, { title: string; about: string; icon: IconName }> = {
  together: { title: 'Watch together', about: 'Everyone here watches these. If someone is behind, start at their episode.', icon: 'users' },
  some: { title: 'Some of you could watch', about: 'Not everyone here watches these.', icon: 'user' },
  waiting: { title: 'Waiting for someone', about: 'Someone who watches this isn’t here. Watching on would leave them behind.', icon: 'clock' },
  new: { title: 'Not started yet', about: 'Nobody here has started these.', icon: 'plus' },
}

// Sections that can get long list a few shows, and the rest on request.
const few: Partial<Record<WatchState, number>> = { some: 4, new: 6 }

// What the people selected can watch now, sorted into what it takes: shows
// all of them watch, shows some of them watch, shows held up by someone who
// is away and shows yet to start (together.ts). A line per show says who has
// seen how much of it and where that leaves me; it opens the show, a person
// on it their episodes, and its buttons tick the next episode.
export function WatchNext({ anime, here, people, progress, self, admin, playing, layout, heading: Heading = 'h2', modeOf, onShow, onOpen, onMark }: {
  anime: WatchAnime[]
  here: WatchPerson[]
  people: WatchPerson[] // everyone, for those who are not here
  progress: WatchProgress[]
  self: string // my username
  admin: boolean
  playing?: number // the anime being watched right now: first in its section, and marked
  layout?: unknown // a change sorts the shows into their sections again
  heading?: 'h2' | 'h3' // the sections' headings: h3 in a window, whose title is h2
  modeOf: (animeId: number, username: string) => WatchMode
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
  const options = watchOptions(anime, here.map((p) => p.username), progress, modeOf)
  // A show stays where it stood when these people were picked, or the list
  // was last laid out (layout): ticking an episode on its line changes what
  // the line says, and moves nothing under the pointer. Dropping a show, or
  // closing a window over the list, lays it out again.
  const who = `${here.map((p) => p.username).join(' ')}|${String(layout)}`
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

  // In a sentence, after its start: "you", not "You".
  const inText = (username: string) => (username === self ? 'you' : name(username))
  // A face with a ring in the person's colour (names stay neutral, so that
  // colours keep meaning a state).
  const face = (username: string, size: number) => (
    <span class={styles.ring} style={{ '--ring': person.get(username)?.nameColor ?? 'var(--border-strong)' }}>
      <Avatar src={person.get(username)?.avatarUrl} size={size} />
    </span>
  )

  // The verdict, in a few words: who it is ready for, or what holds it up.
  // The chips say how far each one is.
  const status = (o: WatchOption) => {
    const watching = o.watching
    const behind = o.behind.map((b) => `${names([b.username], 'is', 'are')} ${b.by} behind`).join(', ')
    if (o.state === 'new') return o.group.length > 0 ? `${names(o.group, 'watches', 'watch')} it` : 'Nobody here has started it'
    if (o.state === 'waiting') return [`Waiting for ${list(o.waiting.map(name))}`, behind].filter(Boolean).join(' · ')
    if (behind) return behind
    if (watching.length === 1) return watching[0] === self ? 'Only you watch this' : `Only ${name(watching[0])} watches this`
    return watching.length + o.casual.length === here.length ? `Ready for all ${here.length} of you` : `Ready for ${list(watching.map(inText))}`
  }

  // Whose next episode the column shows: everyone's, or the one person's
  // who has yet to see it.
  const whoseNext = (o: WatchOption) =>
    o.needs.length !== 1 || o.needs.length === o.watching.length ? 'Next' : o.needs[0] === self ? 'Your next' : `${name(o.needs[0])}’s next`

  // Marking the next episode: for me, and as an admin for everyone it is new
  // to. Where it can be watched now one of them is filled: for an admin the
  // group's, as a room ticks an episode for everyone at once. The visible
  // label starts the accessible name.
  const buttons = (o: WatchOption) => {
    const mine = o.needs.includes(self), others = admin && o.needs.some((u) => u !== self)
    const now = o.state === 'together' || o.state === 'some'
    const of = `episode ${o.next} of ${o.anime.title}`
    const who = list(o.needs.map(inText))
    const forAll = o.needs.length > 2 ? (o.needs.length === here.length ? `Mark for all ${o.needs.length}` : `Mark for ${o.needs.length}`)
      : `Mark for ${o.needs.map((u) => (u === self ? 'me' : name(u))).join(' & ')}`
    return <>
      {others && (
        <Button size="sm" icon={mine ? 'users' : 'check'} variant={now ? 'primary' : 'secondary'} aria-label={`${forAll}: ${of}, for ${who}`}
          title={`Mark episode ${o.next} as watched for ${who}`} onClick={() => onMark(o.anime, o.next, o.needs)}>{forAll}</Button>
      )}
      {mine && (
        <Button size="sm" icon="check" variant={now && !others ? 'primary' : others ? 'ghost' : 'secondary'} aria-label={`${others ? 'Only me' : 'Mark watched'}: ${of}, for you`}
          title={`Mark episode ${o.next} as watched for you`} onClick={() => onMark(o.anime, o.next, [self])}>{others ? 'Only me' : 'Mark watched'}</Button>
      )}
    </>
  }

  const avatarButton = (anime: WatchAnime, u: string, about: string) => {
    const p = person.get(u)
    const label = `${name(u)}, ${about}. Opens ${u === self ? 'your' : 'their'} episodes`
    return (
      <button type="button" key={u} class={styles.face} title={label} aria-label={label}
        disabled={!p} onClick={() => { if (p) onOpen(anime, p) }}>
        {face(u, 20)}
      </button>
    )
  }

  // One line per show: the cover; the title with the verdict and where
  // everyone stands; the next episode and what to do about it.
  const item = (o: WatchOption) => {
    const a = o.anime.id
    const behind = new Map(o.behind.map((b) => [b.username, b.by]))
    const done = here.map((p) => p.username).filter((u) => finished(watchedOf(a, u), o.anime.episodes))
    const dropped = here.map((p) => p.username).filter((u) => modeOf(a, u) === 'dropped' && !done.includes(u))
    const me = (x: string, y: string) => Number(y === self) - Number(x === self)
    // Those here who are level share a summary: their faces and the episode
    // they have all seen up to. Everyone who differs gets a chip: behind
    // (amber, "1 behind"; those equally far behind share one), finished (green, a tick), watching casually (muted,
    // how many they have seen: nobody waits for them). Those not here follow,
    // dashed: first who the show waits on (a clock), then who it does not.
    const following = o.group.filter((u) => present.has(u))
    const level = following.filter((u) => !behind.has(u)).sort(me)
    const behindAt = new Map<number, string[]>() // who is behind, by the episode they are up to
    for (const u of following.filter((u) => behind.has(u)).sort(me)) behindAt.set(seen(a, u), [...(behindAt.get(seen(a, u)) ?? []), u])
    const chips = [
      ...done.sort(me).map((u) => ({ u, kind: 'done' })),
      ...[...o.casual].sort(me).map((u) => ({ u, kind: 'casual' })),
    ]
    const waitingOn = o.waiting.map((u) => ({ u, kind: 'blocking' }))
    const away = o.group.filter((u) => !present.has(u) && !o.waiting.includes(u)).map((u) => ({ u, kind: 'away' }))
    const chip = ({ u, kind }: { u: string; kind: string }) => {
      const p = person.get(u)
      const by = behind.get(u)
      // What the chip shows, then what it means: the visible text starts the name.
      const some = countEpisodes(watchedOf(a, u))
      const shown = kind === 'done' ? `${name(u)} finished` : kind === 'casual' ? `${name(u)} casually${some ? `, seen ${some}` : ''}` : `${name(u)} up to ep ${seen(a, u)}${by ? `, ${by} behind` : ''}`
      const about = kind === 'blocking' ? 'not here and has not seen the next episode: the others wait'
        : kind === 'away' ? 'not here; has seen the next episode, so nobody waits'
        : kind === 'casual' ? 'watches along; nobody waits' : ''
      const label = `${shown}${about ? `, ${about}` : ''}. Opens ${u === self ? 'your' : 'their'} episodes`
      return (
        <button type="button" class={`${styles.person} ${styles[kind]}`} title={label} aria-label={label}
          disabled={!p} onClick={() => { if (p) onOpen(o.anime, p) }}>
          {kind === 'blocking' && <Icon name="clock" size={14} class={styles.clock} />}
          {face(u, 18)}
          <span class={styles.personName}>{name(u)}</span>
          {kind === 'done' ? <span class={styles.at}><Icon name="check" size={13} />finished</span>
            : kind === 'casual' ? <span class={styles.at}>casually{some ? ` · seen ${some}` : ''}</span>
            : <span class={styles.at}>up to ep {seen(a, u)}{by ? <span class={styles.by}>{by} behind</span> : null}</span>}
        </button>
      )
    }
    const behindChips = [...behindAt].sort(([x], [y]) => y - x).map(([episode, us]) => us.length === 1 ? chip({ u: us[0], kind: 'behind' }) : (
      <span class={`${styles.crowd} ${styles.behind}`}>
        <span class={styles.faces}>{us.map((u) => avatarButton(o.anime, u, `up to episode ${episode}, ${behind.get(u)} behind`))}</span>
        <span class={styles.personName}>{us.length > 3 ? `${name(us[0])} +${us.length - 1}` : list(us.map(name))}</span>
        <span class={styles.at}>up to ep {episode}<span class={styles.by}>{behind.get(us[0])} behind</span></span>
      </span>
    ))
    // Those not here, under a label that never parts from its first chip.
    // Several at the same episode share one, as those here who are level do.
    const absentGroup = (label: string, entries: { u: string; kind: string }[]) => {
      const byEpisode = new Map<number, string[]>()
      for (const { u } of entries) byEpisode.set(seen(a, u), [...(byEpisode.get(seen(a, u)) ?? []), u])
      const kind = entries[0]?.kind
      const items = [...byEpisode].map(([episode, us]) => us.length === 1 ? chip({ u: us[0], kind }) : (
        <span class={`${styles.crowd} ${styles[kind]}`}>
          {kind === 'blocking' && <Icon name="clock" size={14} class={styles.clock} />}
          <span class={styles.faces}>{us.map((u) => avatarButton(o.anime, u, kind === 'blocking'
            ? `up to episode ${episode}, not here and has not seen the next episode: the others wait`
            : `up to episode ${episode}, not here; has seen the next episode, so nobody waits`))}</span>
          <span class={styles.personName}>{us.length > 3 ? `${name(us[0])} +${us.length - 1}` : list(us.map(name))}</span>
          <span class={styles.at}>up to ep {episode}</span>
        </span>
      ))
      return items.length > 0 && (
        <li class={styles.group}>
          <ul aria-label={label}>
            <li class={styles.groupStart}><span class={styles.groupLabel} aria-hidden="true">{label}:</span>{items[0]}</li>
            {items.slice(1).map((item, i) => <li key={i}>{item}</li>)}
          </ul>
        </li>
      )
    }
    const moved = o.state !== place(o).state
    const now = o.state === 'together' || o.state === 'some'
    const length = o.anime.episodes !== null ? `of ${o.anime.episodes}` : 'airing'
    return (
      <li key={a} class={`${styles.item} ${styles[o.state]} ${a === playing ? styles.playing : ''}`}>
        {/* The title opens the show too: the cover is for the mouse only. */}
        <button type="button" class={styles.coverButton} tabIndex={-1} aria-hidden="true" onClick={() => onShow(o.anime)}>
          <Cover anime={o.anime} size={72} />
        </button>
        <div class={styles.body}>
          <button type="button" class={styles.titleButton} title="Show details" onClick={() => onShow(o.anime)}>
            <strong class={styles.title}>{o.anime.title}</strong>
            {a === playing && <span class={styles.now}>Watching now</span>}
          </button>
          <span class={styles.status}>
            {status(o)}
            {moved && <span class={styles.moved}> · now: {sections[o.state].title.toLowerCase()}</span>}
          </span>
          <ul class={styles.people} aria-label="Where everyone stands">
            {level.length > 0 && (
              <li class={styles.level}>
                <span class={styles.faces}>{level.map((u) => avatarButton(o.anime, u, `up to episode ${seen(a, u)}`))}</span>
                <span class={styles.levelText}>{level.length > 3 ? `${name(level[0])} +${level.length - 1}` : level.length === here.length && level.length > 2 ? `All ${level.length}` : list(level.map(name))}</span>
                <span class={styles.at}>up to ep {seen(a, level[0])}</span>
              </li>
            )}
            {behindChips.map((c, i) => <li key={i}>{c}</li>)}
            {chips.map((c) => <li key={c.u}>{chip(c)}</li>)}
            {absentGroup('Waiting on', waitingOn)}
            {absentGroup('Away', away)}
            {dropped.length > 0 && <li class={styles.notes}>Dropped: {list(dropped.map(name))}</li>}
          </ul>
        </div>
        <div class={styles.foot}>
          <p class={`${styles.nextEp} ${now ? '' : styles.held}`}>
            <small>{whoseNext(o)}</small>
            <strong><span class={styles.hidden}>episode </span>Ep {o.next}</strong>
            <small class={styles.of}>{length}</small>
            {o.state === 'waiting' && <small class={styles.hold}><Icon name="clock" size={12} />on hold</small>}
          </p>
          <div class={styles.side}>{buttons(o)}</div>
        </div>
      </li>
    )
  }

  return <>
    {watchStates.map((state) => {
      // The show that plays comes first where it stands.
      const found = options.filter((o) => place(o).state === state)
        .sort((a, b) => Number(b.anime.id === playing) - Number(a.anime.id === playing) || place(a).index - place(b).index)
      if (found.length === 0) return null
      const limit = few[state] ?? found.length
      const shown = all[state] ? found : found.slice(0, limit)
      return (
        <section key={state} class={styles.section} aria-label={sections[state].title}>
          <Heading class={styles.heading}><Icon name={sections[state].icon} size={16} class={`${styles.dot} ${styles[state]}`} />{sections[state].title}<span class={styles.count}>{found.length}</span></Heading>
          <p class={styles.about}>{sections[state].about}</p>
          <ul class={styles.list}>{shown.map(item)}</ul>
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
