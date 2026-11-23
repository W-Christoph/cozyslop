// JSON API client. Errors carry the server's user-facing message.

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
  }
}

let onUnauthorized: ((path: string) => void) | undefined

export function setUnauthorizedHandler(handler: (path: string) => void) {
  onUnauthorized = handler
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit = { method, headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] =
      'application/json'
  }

  let res: Response
  try {
    res = await fetch(path, init)
  } catch {
    throw new ApiError(0, 'Could not reach the server.')
  }
  if (res.status === 204) return undefined as T

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    if (res.status === 401 && path !== '/api/auth/login' && path !== '/api/auth/legacy') onUnauthorized?.(path)
    throw new ApiError(
      res.status,
      data?.error ?? `Request failed (${res.status}).`,
    )
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// ---- shared response types (see docs/api.md) --------------------------------

export interface Me {
  username: string
  nickname: string
  nameColor: string
  avatarUrl: string
  admin: boolean
  verified: boolean
  watchlogOff: boolean // left the watchlog: not followed, not on it for anyone
}

export interface ServerSettings {
  message: string
  registration: 'open' | 'invite'
  sourceUrl: string // where to get this server's source code (AGPL)
  watchlog: boolean // anime tracking is on
}

// ---- watchlog: who has watched which episodes of which anime -----------------

export interface WatchAnime {
  id: number // AniList media id
  title: string // romaji
  titleEnglish: string // "" if none
  episodes: number | null // null while unknown (airing)
  coverUrl: string
  addedAt?: number // unix seconds; not in search results
  year?: number | null // search results only
  watchUrl?: string // an episode's page with {ep} for its number ({ep+1} where the site counts one more); "" if nobody knows. Not in search results
  pageUrl?: string // the page the latest episode played on in a room, and which one; "" and 0 if none has
  pageEpisode?: number
  nextEpisode: number | null // the next to air, and when (unix seconds); null if none is announced
  nextAiringAt: number | null
}

export interface WatchPerson { username: string; nickname: string; nameColor: string; avatarUrl: string }

// Episodes as ranges, in order: 1 to 4 and 6 is [[1, 4], [6, 6]].
export type WatchRange = [number, number]

export interface WatchProgress {
  username: string
  animeId: number
  watched: WatchRange[]
  watchedAt: number // when they watched one last, unix seconds
}

// Someone who is not following a show any more: the others do not wait.
// following: the others wait for them (the usual way, and not listed).
// casual: they watch along when they are there. dropped: they have stopped.
export type WatchMode = 'following' | 'casual' | 'dropped'
export interface WatchFollow { username: string; animeId: number; mode: WatchMode }

// A name that means a show: the title part of a file's or a page's name.
// What plays under it is recognised as that show.
export interface WatchAlias { alias: string; animeId: number }

export interface Watchlog { anime: WatchAnime[]; people: WatchPerson[]; progress: WatchProgress[]; follows: WatchFollow[]; aliases: WatchAlias[] }

export const watchlog = {
  get: () => api.get<Watchlog>('/api/watchlog'),
  search: (q: string) => api.get<WatchAnime[]>(`/api/watchlog/search?q=${encodeURIComponent(q)}`),
  add: (id: number) => api.post<WatchAnime>('/api/watchlog/anime', { id }),
  remove: (id: number) => api.del<void>(`/api/watchlog/anime/${id}`),
  // Where the show is watched: an episode's page, with {ep} for its number
  // or not; "" for nowhere.
  setAddress: (id: number, url: string) => api.put<WatchAnime>(`/api/watchlog/anime/${id}/address`, { url }),
  unlink: (alias: string) => api.del<void>(`/api/watchlog/aliases/${encodeURIComponent(alias)}`),
  // Marks the episodes from..to as watched, or as not; answers what is
  // watched now.
  setWatched: (animeId: number, username: string, from: number, to: number, watched: boolean) =>
    api.put<WatchProgress>(`/api/watchlog/progress/${animeId}/${encodeURIComponent(username)}`, { from, to, watched }),
  setFollow: (animeId: number, username: string, mode: WatchMode) =>
    api.put<void>(`/api/watchlog/follow/${animeId}/${encodeURIComponent(username)}`, { mode }),
}

export interface RoomInfo {
  name: string
  access: 'public' | 'account' | 'verified' | 'invite'
  userCount: number
  open: boolean
  online: boolean // the room's desktop is connected
}

export interface AdminUser extends Me {
  disabled: boolean
  createdAt: number
}

export interface AdminRoom {
  container?: 'running' | 'stopped'
  name: string
  source: 'configured' | 'registered' | 'paired' // paired: on another computer, through the tunnel
  connected: boolean
  offlineSince?: number // unix ms; set while viewers are told the desktop is offline
  hibernating?: boolean // the desktop's programs are paused: nobody has been in the room for a while
  userCount: number
}

export interface RoomToken extends AdminRoom {
  nekoToken: string
}

export const adminRooms = {
  start: (name: string) => api.post<AdminRoom>(`/api/admin/rooms/${encodeURIComponent(name)}/start`),
  stop: (name: string) => api.post<AdminRoom>(`/api/admin/rooms/${encodeURIComponent(name)}/stop`),
  list: () => api.get<AdminRoom[]>('/api/admin/rooms'),
  create: (name: string, nekoUrl: string) => api.post<RoomToken>('/api/admin/rooms', { name, nekoUrl }),
  changeAddress: (name: string, nekoUrl: string) => api.patch<AdminRoom>(`/api/admin/rooms/${encodeURIComponent(name)}`, { nekoUrl }),
  newToken: (name: string) => api.post<RoomToken>(`/api/admin/rooms/${encodeURIComponent(name)}/token`),
  remove: (name: string) => api.del<void>(`/api/admin/rooms/${encodeURIComponent(name)}`),
}

// A computer asking to run a room (docs/home-hosting.md, "Pairing").
export interface PairingRequest {
  id: string
  name: string // proposed room name
  code: string // the computer shows the same one
  ip: string
  createdAt: number // unix ms
  expiresAt: number
}

export const adminPairing = {
  list: () => api.get<PairingRequest[]>('/api/admin/pairing'),
  accept: (id: string, target: { name: string } | { replace: string }) =>
    api.post<AdminRoom>(`/api/admin/pairing/${encodeURIComponent(id)}/accept`, target),
  reject: (id: string) => api.del<void>(`/api/admin/pairing/${encodeURIComponent(id)}`),
}

export interface Permission {
  room: string
  username: string
  remote: boolean
  image: boolean
  upload: boolean
  trusted: boolean
  invited: boolean
  banned: boolean
  inviteName: string
  bannedUntil: number | null
}

// What an admin gave an anonymous user in a room (they have no account to
// keep a permission on); lasts while they are in the room.
export interface AnonGrant {
  key: string // identity key, "a:<anon id>"
  remote: boolean
  upload: boolean
}

export interface InviteView {
  code: string
  room: string
  temporary: boolean
  name: string
  remote: boolean
  image: boolean
  upload: boolean
  uses: number
  maxUses: number | null
  expiresAt: number | null
  createdAt: number
  valid: boolean
  path: string
}

export interface PasswordResetLink {
  token: string
  path: string
  expiresAt: number
}

export interface PasswordResetCheck {
  valid: boolean
  username: string
}

// ---- diagnostics: what the server kept for troubleshooting -------------------

export type DiagKind = 'event' | 'request' | 'outbound' | 'sample' | 'stall' | 'boot' | 'shutdown'

export interface DiagEntry {
  id: number
  time: number // unix ms
  kind: DiagKind
  level: string // DEBUG, INFO, WARN, ERROR (or e.g. ERROR+2)
  room?: string
  msg: string
  attrs?: Record<string, unknown>
}

export interface DiagSummary {
  enabled: boolean
  retentionDays: number
  sampleDays: number
  sizeBytes: number
  dropped: number
  tlsHandshakeErrors: number
  since: number
  until: number
  bucketMs: number
  counts: {
    errors: number
    warnings: number
    stalls: number
    requests: number
    failedRequests: number
    anilist: number
    anilistFailed: number
    anilistLimited: number // answered 429: too many
    anilistPeak: number // the most calls in one minute
    anilistSentBytes: number
    anilistReceivedBytes: number
    boots: number
    crashes: number
  }
  lastBoot: DiagEntry | null
  boots: DiagEntry[] // from since, newest first
  anilist: DiagAniList[]
  samples: DiagSample[]
}

export interface DiagAniList {
  time: number
  calls: number
  failed: number
  limited: number
  peak: number
  sentBytes: number
  receivedBytes: number
}

// The highest of each health value in a bucket (the lowest free memory);
// null where none was taken.
export interface DiagSample {
  time: number
  cpuPct: number | null
  memoryMb: number | null
  hostFreeMb: number | null
  hostTotalMb: number | null
  people: number | null
  lagMs: number | null
  lockMs: number | null
  dbMs: number | null
  requestMs: number | null
}

// What the list shows: groups of DIAG_GROUPS, and the rest.
export interface DiagFilter {
  groups: string[]
  level: '' | 'warn' | 'error'
  text: string
  room: string
  since: number
  until?: number
}

export const DIAG_GROUPS: { id: string; label: string; kinds: DiagKind[]; target?: string }[] = [
  { id: 'log', label: 'Log', kinds: ['event'] },
  { id: 'requests', label: 'Requests', kinds: ['request'] },
  { id: 'anilist', label: 'AniList', kinds: ['outbound'], target: 'anilist' },
  { id: 'helpers', label: 'Room helpers', kinds: ['outbound'], target: 'helper' },
  { id: 'stalls', label: 'Stalls', kinds: ['stall'] },
  { id: 'starts', label: 'Starts and stops', kinds: ['boot', 'shutdown'] },
]

export const DIAG_PAGE = 100

export const adminDiagnostics = {
  summary: (hours: number) => api.get<DiagSummary>(`/api/admin/diagnostics/summary?hours=${hours}`),
  entries: (f: DiagFilter, before?: number) => {
    const groups = DIAG_GROUPS.filter((g) => f.groups.includes(g.id))
    if (!groups.length) return Promise.resolve([])
    const q = new URLSearchParams({
      kind: [...new Set(groups.flatMap((g) => g.kinds))].join(','),
      since: String(f.since),
      limit: String(DIAG_PAGE),
    })
    const targets = groups.flatMap((g) => g.target ? [g.target] : [])
    if (targets.length) q.set('target', targets.join(','))
    if (f.until) q.set('until', String(f.until))
    if (f.level) q.set('level', f.level)
    if (f.room) q.set('room', f.room)
    if (f.text.trim()) q.set('q', f.text.trim())
    if (before) q.set('before', String(before))
    return api.get<{ entries: DiagEntry[] }>(`/api/admin/diagnostics?${q}`).then((r) => r.entries)
  },
  stalls: (since: number) =>
    api.get<{ entries: DiagEntry[] }>(`/api/admin/diagnostics?kind=stall&since=${since}&limit=200`).then((r) => r.entries),
  // A stall's or crash's dump, as text.
  dump: async (name: string) => {
    let res: Response
    try {
      res = await fetch(dumpUrl(name))
    } catch {
      throw new ApiError(0, 'Could not reach the server.')
    }
    if (!res.ok) {
      const data = await res.json().catch(() => null)
      throw new ApiError(res.status, data?.error ?? `Request failed (${res.status}).`)
    }
    return res.text()
  },
  rooms: () => adminRooms.list().then((list) => list.map((r) => r.name)),
  clear: () => api.del<void>('/api/admin/diagnostics'),
}
export type DiagnosticsSource = typeof adminDiagnostics

export const dumpUrl = (name: string) => `/api/admin/diagnostics/dumps/${encodeURIComponent(name)}`
