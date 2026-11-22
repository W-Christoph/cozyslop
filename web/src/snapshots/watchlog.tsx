import type { WatchAnime, WatchProgress } from '../api'
import type { WatchPlaying, WatchProposal, WatchViewer } from '../room/protocol'
import { RoomContext } from '../components/room/RoomContext'
import { WatchPanel } from '../components/room/WatchPanel'
import { ShowDialog } from '../components/watchlog/ShowDialog'
import { WatchNext } from '../components/watchlog/WatchNext'
import { fakeRoom, Variants, type Scene } from './scene'
import styles from './Snapshots.module.css'

// The watchlog in a room: the bar of what plays, the question whether you
// watched an episode, and Watch next.

const now = Date.now()

// A made-up cover: a gradient in the hue given.
const cover = (hue: number) => `data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" width="70" height="100"><defs><linearGradient id="g" x2="0" y2="1">` +
  `<stop offset="0" stop-color="hsl(${hue} 55% 55%)"/><stop offset="1" stop-color="hsl(${hue + 40} 45% 25%)"/></linearGradient></defs>` +
  `<rect width="70" height="100" fill="url(#g)"/></svg>`)}`

// The top of the chat, as one in the room sees it.
function Chat({ playing = null, proposal = null }: { playing?: WatchPlaying | null; proposal?: WatchProposal | null }) {
  const room = fakeRoom({ self: { key: 'u:1', anonymous: false }, watchPlaying: playing, watchProposal: proposal })
  return <div class={styles.chat}><RoomContext.Provider value={room}><WatchPanel bar /></RoomContext.Provider></div>
}

const episode: WatchProposal = {
  id: 1, animeId: 154587, title: 'Sousou no Frieren', coverUrl: cover(160), episodes: 28, episode: 9,
  file: '[SubsPlease] Sousou no Frieren - 09 (1080p).mkv', applyAt: now + 107000, viewers: [],
}
const asking = (viewer: Partial<WatchViewer>, change: Partial<WatchProposal> = {}): WatchProposal => ({
  ...episode, ...change,
  viewers: [{ key: 'u:1', nickname: 'You', coverage: 0.94, checked: true, answered: false, earlier: 0, earlierToo: false, ...viewer }],
})

const show = (id: number, title: string, change: Partial<WatchAnime>): WatchAnime => ({
  id, title, titleEnglish: '', episodes: null, coverUrl: cover(id * 70), addedAt: id, nextEpisode: null, nextAiringAt: null, watchUrl: '', ...change,
})
const people = [
  { username: 'me', nickname: 'Me', nameColor: '#7cf', avatarUrl: '' },
  { username: 'mika', nickname: 'Mika', nameColor: '#fc7', avatarUrl: '' },
]
const watched = (animeId: number, username: string, to: number, from = 1): WatchProgress => ({ animeId, username, watched: [[from, to]], watchedAt: animeId })

export const watchlogScenes: Scene[] = [
  {
    name: 'watch-ask',
    about: 'Whether you watched the episode that ended, in its states.',
    width: 1100,
    surface: 'room',
    render: () => (
      <Variants columns={3} items={[
        ['Saw most of ep 9, marked nothing before', <Chat proposal={asking({ earlier: 8 })} />],
        ['Ticked: also mark 1–8', <Chat proposal={asking({ earlier: 8, earlierToo: true })} />],
        ['Some earlier ones not marked', <Chat proposal={asking({ earlier: 3 })} />],
        ['Saw 35%: no unless you say yes', <Chat proposal={asking({ coverage: 0.35, checked: false, earlier: 8 })} />],
        ['Everything before marked', <Chat proposal={asking({})} />],
        ['Episode 2, episode 1 not marked', <Chat proposal={asking({ earlier: 1 }, { episode: 2 })} />],
        ['Show not recognised', <Chat proposal={asking({}, { animeId: 0, title: 'Some Show', episodes: null, applyAt: 0 })} />],
      ]} />
    ),
  },
  {
    name: 'watch-bar',
    about: 'What plays, at the top of the chat.',
    width: 760,
    surface: 'room',
    render: () => (
      <Variants columns={2} items={[
        ['AniList’s guess', <Chat playing={{ animeId: 154587, title: 'Sousou no Frieren', episode: 10, linked: false }} />],
        ['Not recognised', <Chat playing={{ animeId: 0, title: 'Some Show', episode: 3, linked: false }} />],
        ['Linked', <Chat playing={{ animeId: 154587, title: 'Sousou no Frieren', episode: 10, linked: true }} />],
      ]} />
    ),
  },
  {
    name: 'watch-next',
    about: 'Watch next for two people: one show to open where it is watched, one where only the latest episode’s page is known, and shows whose next episode is not out (one with the latest episode to open).',
    width: 860,
    surface: 'page',
    render: () => (
      <WatchNext heading="h3" self="me" here={people} people={people}
        anime={[
          show(1, 'Sousou no Frieren', { episodes: 28, watchUrl: 'https://site.example/sousou-no-frieren-episode-{ep}' }),
          show(2, 'Dandadan Season 2', { nextEpisode: 8, nextAiringAt: now / 1000 + 2 * 86400 + 5 * 3600, pageUrl: 'https://stream.example/watch/G4VUQ1/the-curse', pageEpisode: 7 }),
          show(5, 'Kusuriya no Hitorigoto', { episodes: 24, pageUrl: 'https://stream.example/watch/GX9Q2/the-banquet', pageEpisode: 3 }),
          show(3, 'Kaiju No. 8', { nextEpisode: 5, nextAiringAt: now / 1000 + 3 * 3600 + 20 * 60 }),
          show(4, 'One Piece', { nextEpisode: 1120, nextAiringAt: now / 1000 + 4 * 86400 }),
        ]}
        progress={[watched(1, 'me', 8), watched(1, 'mika', 8), watched(2, 'me', 7), watched(2, 'mika', 7),
          watched(3, 'me', 4), watched(4, 'me', 1100), watched(4, 'mika', 1110), watched(5, 'me', 3), watched(5, 'mika', 3)]}
        modeOf={() => 'following'} onShow={() => {}} onOpen={() => {}} onPlay={() => {}} />
    ),
  },
  ...[
    { name: 'show-window', about: 'A show’s window, where it is watched learned from the room’s Firefox: the address can be changed here.', address: 'https://site.example/sousou-no-frieren-episode-{ep}' },
    { name: 'show-window-no-address', about: 'A show’s window before anyone watched it in Firefox: an episode’s page is pasted here.', address: '' },
    { name: 'show-window-page', about: 'A show’s window for a site whose addresses do not say the episode: only the latest episode’s page, to open again.', address: '', page: 'https://stream.example/watch/G4VUQ1/the-journeys-end' },
  ].map(({ name, about, address, page }): Scene => ({
    name, about, width: 900, surface: 'page', window: { height: 1150 },
    render: () => (
      <ShowDialog self="me" playing={false} busy={false} mayRemove={false} mayChange={(u) => u === 'me'}
        anime={show(2, 'Sousou no Frieren', { titleEnglish: 'Frieren: Beyond Journey’s End', nextEpisode: 10, nextAiringAt: now / 1000 + 86400 * 3 + 3600 * 4, watchUrl: address, pageUrl: page ?? '', pageEpisode: page ? 9 : 0 })}
        people={people} watchedOf={(u) => (u === 'me' ? [[1, 8]] : [[1, 9]])} modeOf={() => 'following'}
        names={address ? ['sousou no frieren'] : []} onEpisodes={() => {}} onSet={() => {}} onFollow={() => {}}
        onAddress={async () => {}} onPlay={() => {}} onRefresh={() => {}} onRemove={() => {}} onClose={() => {}} />
    ),
  })),
]
