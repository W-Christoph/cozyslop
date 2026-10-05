import { effect } from '@preact/signals'
import { useLayoutEffect, useState } from 'preact/hooks'
import type { ChatMessage } from '../../room/protocol'
import { useRoomStore } from './RoomContext'

export function useUnreadChat(chatOpen: boolean) {
  const store = useRoomStore()
  const [unread, setUnread] = useState(0)
  useLayoutEffect(() => {
    if (chatOpen) setUnread(0)
    let previous: readonly ChatMessage[] = []
    let initialized = false
    let previousSelf = store.self.value
    return effect(() => {
      const messages = store.chat.value
      const user = store.self.value
      if (user !== previousSelf) { initialized = false; previousSelf = user }
      const self = user?.key
      if (store.server.value !== 'connected' || !self) { initialized = false; return }
      // Appends keep the existing messages; a welcome snapshot replaces them,
      // even when it has the same IDs followed by newer history.
      const appended = initialized && messages.length > previous.length && previous.every((message, i) => messages[i] === message)
      if (!chatOpen && appended) {
        const count = messages.slice(previous.length).filter((message) => message.author !== self).length
        if (count) setUnread((value) => value + count)
      }
      initialized = true
      previous = messages
    })
  }, [store, chatOpen])
  return chatOpen ? 0 : unread
}
