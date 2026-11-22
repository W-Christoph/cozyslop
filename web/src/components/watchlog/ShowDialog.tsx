import { useEffect, useRef, useState } from 'preact/hooks'
import type { WatchAnime, WatchMode, WatchPerson, WatchRange } from '../../api'
import { Button, ButtonLink } from '../Button'
import { Modal } from '../Modal'
import { Avatar } from '../ui/Avatar'
import { Badge } from '../ui/Badge'
import { Input } from '../ui/Field'
import { Icon } from '../ui/Icon'
import { Notice } from '../ui/Notice'
import { Cover } from './AnimeSearch'
import { airsIn, airsOn, latestPage } from './airing'
import { EpisodeBar } from './EpisodeBar'
import { FollowSelect } from './FollowSelect'
import { countEpisodes, finished, firstMissing } from './episodes'
import styles from './ShowDialog.module.css'

// One show with everything to do about it: who has watched how far, with
// the next episode to tick and how they follow it for each person one may
// change;
// when its next episode airs, where it is watched, with its latest episode
// to open there (as Watch next opens the next); and the show itself, to
// bring up to date from AniList or take off the list.
export function ShowDialog({ anime, people, self, playing, busy, watchedOf, modeOf, mayChange, mayRemove, names, links, onUnlink, onEpisodes, onSet, onFollow, onAddress, onPlay, onRefresh, onRemove, onClose }: {
  anime: WatchAnime
  people: WatchPerson[] // me, those selected and everyone who has watched some of it
  self: string // my username
  playing: boolean // the room is watching it
  busy: boolean // AniList is being asked
  watchedOf: (username: string) => readonly WatchRange[]
  modeOf: (username: string) => WatchMode
  mayChange: (username: string) => boolean
  mayRemove: boolean
  names: string[] // what plays under these names is recognised as this show
  links?: boolean // without onPlay, an episode opens in a tab of this browser
  onUnlink?: (name: string) => void // admins: forget one linked by mistake
  onEpisodes: (person: WatchPerson) => void
  onSet: (username: string, episode: number) => void // ticks one episode
  onFollow: (username: string, mode: WatchMode) => void
  onAddress: (url: string) => Promise<void> // where it is watched; throws what is wrong with it
  onPlay?: (episode: number) => void // opens the episode in the room's Firefox
  onRefresh: () => void
  onRemove: () => void
  onClose: () => void
}) {
  // A second click right after the first is not meant for the next episode.
  const last = useRef(0)
  const tick = (username: string, episode: number) => {
    if (Date.now() - last.current < 600) return
    last.current = Date.now()
    onSet(username, episode)
  }
  const [address, setAddress] = useState(anime.watchUrl ?? '')
  const [saving, setSaving] = useState(false)
  const [addressError, setAddressError] = useState('')
  // Learned in a room meanwhile, or saved as the server writes it.
  useEffect(() => { setAddress(anime.watchUrl ?? '') }, [anime.watchUrl])
  async function saveAddress(e: Event) {
    e.preventDefault()
    setSaving(true)
    setAddressError('')
    try {
      await onAddress(address.trim())
    } catch (err) {
      setAddressError(err instanceof Error ? err.message : 'Something went wrong.')
    } finally {
      setSaving(false)
    }
  }
  const airs = anime.nextEpisode !== null && anime.nextAiringAt !== null ? { episode: anime.nextEpisode, at: anime.nextAiringAt } : null
  const latest = onPlay || links ? latestPage(anime) : null
  const site = latest ? hostOf(latest.address) : ''
  return (
    <Modal title={anime.title} size="md" onClose={onClose}>
      <div class={styles.head}>
        <Cover anime={anime} size={96} />
        <div class={styles.about}>
          {anime.titleEnglish && anime.titleEnglish.toLowerCase() !== anime.title.toLowerCase() && <strong class={styles.title}>{anime.titleEnglish}</strong>}
          <span class={styles.length}>
            {anime.episodes !== null ? `${anime.episodes} episode${anime.episodes === 1 ? '' : 's'}` : 'Length not known yet (still airing)'}
            {playing && <Badge tone="accent" icon="play">Watching now</Badge>}
          </span>
          {airs && (
            <span class={styles.length}>
              <Icon name="bell" size={14} />
              {airs.at * 1000 > Date.now() ? <span>Episode {airs.episode} airs {airsOn(airs.at)} · {airsIn(airs.at)}</span> : <span>Episode {airs.episode} is out</span>}
            </span>
          )}
        </div>
      </div>
      <h3 class={styles.label}>Who has watched what</h3>
      <ul class={styles.people}>
        {people.map((p) => {
          const watched = watchedOf(p.username), count = countEpisodes(watched), next = firstMissing(watched)
          const done = finished(watched, anime.episodes), mode = modeOf(p.username), dropped = mode === 'dropped', editable = mayChange(p.username)
          const me = p.username === self
          return (
            <li key={p.username} class={`${styles.person} ${dropped && !done ? styles.dropped : ''}`}>
              <Avatar src={p.avatarUrl} size={28} />
              <div class={styles.progress}>
                <span class={styles.name}>
                  <span style={{ color: p.nameColor }}>{me ? 'You' : p.nickname}</span>
                  {done ? <Badge tone="success" icon="check">Finished</Badge> : !editable && mode !== 'following' ? <Badge title={`The others do not wait for ${p.nickname}`}>{mode === 'casual' ? 'Casual' : count === 0 ? 'Not interested' : 'Dropped'}</Badge> : null}
                  {/* At the line's end, so that its changing label moves nothing. */}
                  {editable && !done && <span class={styles.drop}><FollowSelect mode={mode} who={me ? 'you' : p.nickname} started={count > 0} onChange={(m) => onFollow(p.username, m)} /></span>}
                </span>
                <EpisodeBar watched={watched} episodes={anime.episodes} />
                <span class={styles.count}>{count === 0 ? 'Not started' : `${count === next - 1 ? '' : `Up to ep ${next - 1} · `}${count}${anime.episodes !== null ? ` / ${anime.episodes}` : ''} watched`}</span>
              </div>
              <div class={styles.actions}>
                {editable && !done && <Button size="sm" icon="check" class={styles.watched} title={`Mark episode ${next} as watched for ${me ? 'you' : p.nickname}`} aria-label={`Mark ep ${next}: as watched for ${me ? 'you' : p.nickname}`} onClick={() => tick(p.username, next)}>Mark ep {next}</Button>}
                <Button size="sm" variant="ghost" title="Tick and untick single episodes" onClick={() => onEpisodes(p)}>{editable ? 'Episodes…' : 'Episodes'}</Button>
              </div>
            </li>
          )
        })}
      </ul>
      {names.length > 0 && <>
        <h3 class={styles.label}>Recognised as</h3>
        <p class={styles.note}>A file or page that plays under one of these names counts as this show.</p>
        <ul class={styles.names}>
          {names.map((name) => (
            <li key={name} class={styles.linked}>
              <span class={styles.linkedName}>{name}</span>
              {onUnlink && <Button size="sm" variant="ghost" title="Forget this name: what plays under it is matched anew" aria-label={`Unlink ${name}`} onClick={() => onUnlink(name)}>Unlink</Button>}
            </li>
          ))}
        </ul>
      </>}
      <h3 class={styles.label}>Where to watch</h3>
      {latest && (
        <div class={styles.latest}>
          <span class={styles.latestText}>Episode {latest.episode} on <span class={styles.site} title={latest.address}>{site}</span></span>
          {onPlay
            ? <Button size="sm" icon="play" title={`Open episode ${latest.episode} in the room’s Firefox, for everyone here`} onClick={() => onPlay(latest.episode)}>Open ep {latest.episode}</Button>
            : <ButtonLink size="sm" icon="play" title={latest.address} href={latest.address} target="_blank" rel="noreferrer noopener">Open ep {latest.episode}</ButtonLink>}
        </div>
      )}
      <form class={styles.address} onSubmit={(e) => { void saveAddress(e) }}>
        <Input type="url" value={address} placeholder="https://…/show-episode-{ep}" aria-label="Address of an episode’s page"
          onInput={(e) => setAddress(e.currentTarget.value)} />
        <Button size="sm" type="submit" disabled={saving || address.trim() === (anime.watchUrl ?? '')}>{saving ? 'Saving…' : 'Save'}</Button>
      </form>
      {addressError && <Notice tone="error" compact class={styles.addressError}>{addressError}</Notice>}
      <p class={styles.note}>
        The page of any episode, or one with <code>{'{ep}'}</code> where the number goes. It is learned by itself when an episode plays in a room’s Firefox;
        Watch next then opens the next episode there. Where a site’s addresses don’t say the episode, only the page of the latest one played is known.
      </p>
      <h3 class={styles.label}>Manage the show</h3>
      <div class={styles.manage}>
        <ButtonLink size="sm" icon="link" href={`https://anilist.co/anime/${anime.id}`} target="_blank" rel="noreferrer noopener">Open on AniList</ButtonLink>
        <Button size="sm" icon="refresh" disabled={busy} title="Fetch the title, cover and number of episodes again" onClick={onRefresh}>{busy ? 'Updating…' : 'Update from AniList'}</Button>
        {mayRemove && <Button size="sm" variant="danger-outline" icon="trash" class={styles.remove} onClick={onRemove}>Remove from watchlog</Button>}
      </div>
      {!mayRemove && <p class={styles.note}>Only admins can remove a show someone has watched.</p>}
    </Modal>
  )
}

// The site of an address: "site.example".
function hostOf(address: string) {
  try {
    return new URL(address).host
  } catch {
    return address
  }
}
