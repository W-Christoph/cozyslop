import { effect } from '@preact/signals'
import { useEffect } from 'preact/hooks'
import { pageTitle, preferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { unreadFavicon } from './unreadFavicon'

export function useRoomPresence() {
  const store = useRoomStore()
  useEffect(() => {
    const favicon = unreadFavicon()
    let unread = 0
    let previous: readonly number[] = []
    let initialized = false
    const visibility = () => {
      store.setActive(!document.hidden)
      if (!document.hidden) { unread = 0; favicon.setCount(0) }
    }
    document.addEventListener('visibilitychange', visibility)
    const stopTitle = effect(() => { pageTitle.value = store.settings.value?.name ?? store.room })
    const stopActivity = effect(() => {
      // Resend after welcome/reconnect, when presence defaults are replaced.
      if (store.server.value === 'connected' && store.selfKey.value) store.setActive(!document.hidden)
    })
    const stopMuted = effect(() => {
      const { muted, volume, showIfMuted } = preferences.value
      const listeningOff = store.paused.value || muted || volume === 0
      if (store.server.value === 'connected' && store.selfKey.value) store.setMuted(showIfMuted && listeningOff)
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
      favicon.dispose()
      pageTitle.value = null
    }
  }, [store])
}
