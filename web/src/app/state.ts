// App-wide state: who is logged in, server settings and per-browser
// preferences. One instance, imported directly.

import { computed, effect, signal } from '@preact/signals'
import { api, type Me, type ServerSettings } from '../api'

export type Theme = 'default' | 'legacy' | 'light'

// Per-browser preferences, persisted in localStorage.
export interface Preferences {
  theme: Theme
  volume: number // 0-100
  muted: boolean
  muteChatNotification: boolean
  showUsernames: boolean
  transparentChat: boolean
  showLeaveJoinMsg: boolean
  showIfMuted: boolean
  titleNameInFront: boolean
  userlistOnLeft: boolean
  smallPfp: boolean
  manualLoadMedia: boolean
  audioOnly: boolean
}

const defaultPreferences: Preferences = {
  theme: 'default',
  volume: 100,
  muted: false,
  muteChatNotification: true,
  showUsernames: true,
  transparentChat: true,
  showLeaveJoinMsg: true,
  showIfMuted: true,
  titleNameInFront: false,
  userlistOnLeft: false,
  smallPfp: false,
  manualLoadMedia: false,
  audioOnly: false,
}

const PREFS_KEY = 'preferences'

function loadPreferences(): Preferences {
  try {
    const stored = JSON.parse(localStorage.getItem(PREFS_KEY) ?? '{}')
    return { ...defaultPreferences, ...(stored && typeof stored === 'object' ? stored : {}) }
  } catch {
    return { ...defaultPreferences }
  }
}

export const me = signal<Me | null>(null)
export const meLoaded = signal(false)
export const loggedIn = computed(() => me.value !== null)
export const serverSettings = signal<ServerSettings>({ message: '', registration: 'invite' })
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

effect(() => {
  document.documentElement.dataset.theme = preferences.value.theme
})

effect(() => {
  const title = pageTitle.value
  if (!title) document.title = 'CozyCast - Movie night over the internet'
  else document.title = preferences.value.titleNameInFront ? `CozyCast: ${title}` : `${title} - CozyCast`
})

export async function refreshMe() {
  try {
    const res = await api.get<{ user: Me | null }>('/api/me')
    me.value = res.user
  } finally {
    meLoaded.value = true
  }
}

export async function refreshServerSettings() {
  serverSettings.value = await api.get<ServerSettings>('/api/settings')
}

export async function logout() {
  await api.post('/api/auth/logout')
  me.value = null
}

// An invite code waiting for the user to log in or register. Kept in
// sessionStorage so it survives the detour through the login page.
const PENDING_INVITE_KEY = 'pendingInvite'

export const pendingInvite = {
  get: () => sessionStorage.getItem(PENDING_INVITE_KEY),
  set: (code: string) => sessionStorage.setItem(PENDING_INVITE_KEY, code),
  clear: () => sessionStorage.removeItem(PENDING_INVITE_KEY),
}
