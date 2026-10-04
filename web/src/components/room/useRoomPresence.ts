import { effect } from '@preact/signals'
import { useEffect } from 'preact/hooks'
import { pageTitle, preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { unreadFavicon } from './unreadFavicon'

// A tab hidden for this long counts as away, as in the old CozyCast.
const AWAY_AFTER_MS = 5 * 60 * 1000

export function useRoomPresence() {
  const store = useRoomStore()
  useEffect(() => {
    const favicon = unreadFavicon()
    let unread = 0
    let previous: readonly number[] = []
    let initialized = false
    let active = true
    let awayTimer: number | undefined
    const visibility = () => {
      if (!document.hidden) {
        window.clearTimeout(awayTimer)
        awayTimer = undefined
        unread = 0
        favicon.setCount(0)
        if (!active) { active = true; store.setActive(true) }
      } else if (awayTimer === undefined) {
        awayTimer = window.setTimeout(() => { awayTimer = undefined; active = false; store.setActive(false) }, AWAY_AFTER_MS)
      }
    }
    document.addEventListener('visibilitychange', visibility)
    visibility() // a room opened in a background tab
    const stopTitle = effect(() => { pageTitle.value = store.settings.value?.name ?? store.room })
    const stopActivity = effect(() => {
      // Resend after welcome/reconnect, when presence defaults are replaced.
      if (store.server.value === 'connected' && store.selfKey.value) store.setActive(active)
    })
    // What the server was last told; null after a welcome/reconnect, which
    // replaces presence with its defaults.
    let sentMuted: boolean | null = null
    const stopMuted = effect(() => {
      const { muted, volume, showIfMuted } = preferences.value
      const listeningOff = store.paused.value || muted || volume === 0
      if (store.server.value !== 'connected' || !store.selfKey.value) { sentMuted = null; return }
      // Only changes: dragging the volume slider updates the preferences many
      // times a second, and every message counts against the rate limit.
      const value = showIfMuted && listeningOff
      if (value !== sentMuted) { sentMuted = value; store.setMuted(value) }
    })
    const stopChat = effect(() => {
      const messages = store.chat.value
      const self = store.selfKey.value
      const connected = store.server.value === 'connected'
      if (!connected || !self) { initialized = false; return }
      const ids = messages.map((message) => message.id)
      // Only appends count. A replaced welcome/history snapshot is a baseline;
      // edits and deletions never create unread messages.
      const appended = initialized && ids.length > previous.length && previous.every((id, i) => ids[i] === id)
      if (document.hidden && appended) {
        unread += messages.slice(previous.length).filter((message) => message.author !== self).length
        favicon.setCount(unread)
      }
      initialized = true
      previous = ids
    })
    return () => {
      stopTitle(); stopActivity(); stopMuted(); stopChat()
      document.removeEventListener('visibilitychange', visibility)
      window.clearTimeout(awayTimer)
      favicon.dispose()
      pageTitle.value = null
    }
  }, [store])
}
