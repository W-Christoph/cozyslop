// App-wide state: who is logged in, server settings and per-browser
// preferences. One instance, imported directly.

import { effect, signal } from '@preact/signals'
import { api, ApiError, setUnauthorizedHandler, type Me, type ServerSettings } from '../api'

// 'system' follows the device: light, or the default dark theme.
export type Theme = 'system' | 'default' | 'dark' | 'legacy' | 'light'
export type Accent = 'orange' | 'blurple' | 'blue' | 'teal' | 'green' | 'pink' | 'red'
// classic: CozyCast's bubbles; modern: flat, with room for pictures;
// compact: one line per message.
export type ChatStyle = 'classic' | 'modern' | 'compact'

// Per-browser preferences, persisted in localStorage.
export interface Preferences {
  theme: Theme
  accent: Accent
  chatStyle: ChatStyle
  chatAvatars: boolean
  chatScale: number // percent of the normal text size
  chatWidth: number // percent of the normal sidebar width
  chatMediaHeight: number // tallest picture or video in chat, in px; 0: no limit
  volume: number // 0-100
  muted: boolean
  muteChatNotification: boolean
  showUsernames: boolean
  outlineNames: boolean // names in the user list over the stream, in fullscreen
  transparentChat: boolean
  showLeaveJoinMsg: boolean
  showMuted: boolean // mark others who are muted or paused
  shareMuted: boolean // tell others when this browser is
  titleNameInFront: boolean
  userlistOnLeft: boolean
  smallPfp: boolean
  manualLoadMedia: boolean
  shortcuts: boolean
  audioOnly: boolean
  chatViewOnPause: boolean // chat takes the stream's place while it is paused
  streamPreview: number // seconds between stills of the desktop in the chat view; 0: none
  askBeforePaste: boolean
}

const defaultPreferences: Preferences = {
  theme: 'default',
  accent: 'orange',
  chatStyle: 'classic',
  chatAvatars: false,
  chatScale: 100,
  chatWidth: 100,
  chatMediaHeight: 400,
  volume: 100,
  muted: false,
  muteChatNotification: true,
  showUsernames: true,
  outlineNames: true,
  transparentChat: true,
  showLeaveJoinMsg: true,
  showMuted: true,
  shareMuted: true,
  titleNameInFront: false,
  userlistOnLeft: false,
  smallPfp: false,
  manualLoadMedia: false,
  shortcuts: true,
  audioOnly: false,
  chatViewOnPause: true,
  streamPreview: 5,
  askBeforePaste: true,
}

const PREFS_KEY = 'preferences'

function loadPreferences(): Preferences {
  try {
    const parsed: unknown = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}')
    const stored = parsed && typeof parsed === 'object' ? { ...parsed } as Record<string, unknown> : {}
    // Older versions: one setting for seeing and sharing muted, and three
    // chat widths.
    if (typeof stored.showIfMuted === 'boolean') {
      stored.showMuted ??= stored.showIfMuted
      stored.shareMuted ??= stored.showIfMuted
      delete stored.showIfMuted
    }
    if (typeof stored.chatWidth === 'string') stored.chatWidth = ({ wide: 130, wider: 170 } as Record<string, number>)[stored.chatWidth] ?? 100
    return { ...defaultPreferences, ...stored }
  } catch {
    return { ...defaultPreferences }
  }
}

export const me = signal<Me | null>(null)
export const meLoaded = signal(false)
export const serverSettings = signal<ServerSettings>({
  message: '',
  registration: 'invite',
  sourceUrl: 'https://github.com/W-Christoph/cozyslop',
})
export const preferences = signal<Preferences>(loadPreferences())

// The room's window title part of the browser tab title, set by the room page.
export const pageTitle = signal<string | null>(null)

export function updatePreferences(change: Partial<Preferences>) {
  preferences.value = { ...preferences.value, ...change }
}

effect(() => {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(preferences.value))
  } catch {
    // storage full or blocked: preferences just won't persist
  }
})

const lightScheme = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : undefined
const deviceLight = signal(lightScheme?.matches ?? false)
lightScheme?.addEventListener('change', (e) => { deviceLight.value = e.matches })

export function resolveTheme(theme: Theme): Exclude<Theme, 'system'> {
  return theme === 'system' ? (deviceLight.value ? 'light' : 'default') : theme
}

effect(() => {
  const { theme, accent } = preferences.value
  document.documentElement.dataset.theme = resolveTheme(theme)
  document.documentElement.dataset.accent = accent
})

// The settings window, opened from the header and from inside a room.
export type SettingsSection = 'account' | 'appearance' | 'chat' | 'room' | 'notifications' | 'shortcuts'
export const settingsOpen = signal<SettingsSection | null>(null)

effect(() => {
  const title = pageTitle.value
  if (!title) document.title = 'CozyCast - Movie night over the internet'
  else document.title = preferences.value.titleNameInFront ? `CozyCast: ${title}` : `${title} - CozyCast`
})

let meVersion = 0
const authChannel = typeof BroadcastChannel === 'undefined' ? undefined : new BroadcastChannel('cozycast-auth')

export async function refreshMe() {
  const version = ++meVersion
  try {
    const res = await api.get<{ user: Me | null }>('/api/me')
    if (version === meVersion) me.value = res.user
    if (!res.user) await legacyLogin()
  } catch (e) {
    if (e instanceof ApiError && e.status === 401 && version === meVersion) me.value = null
  } finally {
    meLoaded.value = true
  }
}

// A browser that was logged in to the old CozyCast still holds its refresh
// token. The server trades it for a session, once (docs/migration.md). The
// token is left in place: the old site still knows this browser if the
// migration is rolled back. A token the server has answered for is not sent
// again from this tab.
const LEGACY_TOKEN_KEY = 'refreshToken'
const LEGACY_TRIED_KEY = 'legacyLoginTried'
let legacyTried = false

async function legacyLogin() {
  if (legacyTried) return
  legacyTried = true
  try {
    const token = localStorage.getItem(LEGACY_TOKEN_KEY)
    if (!token || sessionStorage.getItem(LEGACY_TRIED_KEY)) return
    try {
      const res = await api.post<{ user: Me }>('/api/auth/legacy', { token })
      authChanged(res.user)
    } catch (e) {
      // Not answered, or rate limited: try again on the next page load.
      if (!(e instanceof ApiError) || e.status !== 401) return
    }
    sessionStorage.setItem(LEGACY_TRIED_KEY, '1')
  } catch {
    // storage blocked: nothing to carry over
  }
}

setUnauthorizedHandler((path) => {
  // refreshMe handles its own 401; starting another read would loop.
  if (path !== '/api/me') void refreshMe()
})
if (authChannel) authChannel.onmessage = () => { void refreshMe() }
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible') void refreshMe()
})

export async function refreshServerSettings() {
  serverSettings.value = await api.get<ServerSettings>('/api/settings')
}

export async function logout() {
  ++meVersion
  await api.post('/api/auth/logout')
  authChanged(null)
}

export async function login(username: string, password: string) {
  ++meVersion
  const res = await api.post<{ user: Me }>('/api/auth/login', { username, password })
  authChanged(res.user)
}

export async function register(username: string, password: string, inviteCode?: string) {
  ++meVersion
  const res = await api.post<{ user: Me }>('/api/auth/register', {
    username, password, ...(inviteCode ? { inviteCode } : {}),
  })
  authChanged(res.user)
}

function authChanged(user: Me | null) {
  ++meVersion
  me.value = user
  authChannel?.postMessage('changed')
}

// An invite code waiting for the user to log in or register. Kept in
// sessionStorage so it survives the detour through the login page.
const PENDING_INVITE_KEY = 'pendingInvite'

export const pendingInvite = {
  get: () => sessionStorage.getItem(PENDING_INVITE_KEY),
  set: (code: string) => sessionStorage.setItem(PENDING_INVITE_KEY, code),
  clear: () => sessionStorage.removeItem(PENDING_INVITE_KEY),
}
