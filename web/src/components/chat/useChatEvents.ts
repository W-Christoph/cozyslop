import { effect, untracked } from '@preact/signals'
import { useEffect, useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import type { RoomStore } from '../../room/store'
import { pingCount } from './parseMessage'
import { chatChanges, type ChatSnapshot } from './chatChanges'

export interface TemporaryLine {
  id: number
  after: number | null
  time: number
  body: string
}

export function useChatEvents(store: RoomStore): readonly TemporaryLine[] {
  const [lines, setLines] = useState<readonly TemporaryLine[]>([])
  useEffect(() => {
    let previous: ChatSnapshot = { chat: store.chat.peek(), users: store.users.peek(), self: store.self.peek(), connected: store.server.peek() === 'connected' }
    let sequence = 0
    const timers = new Set<number>()
    const play = () => { void new Audio('/audio/pop.wav').play().catch(() => {}) }
    const off = effect(() => {
      const chat = store.chat.value
      const users = store.users.value
      const self = store.self.value
      const connected = store.server.value === 'connected'
      const before = previous
      const next = { chat, users, self, connected }
      const changes = chatChanges(before, next)
      untracked(() => {
        if (changes.welcome) setLines([])
        else if (chat !== before.chat) {
          const present = new Set(chat.map((m) => m.id))
          // Keep a join/leave line in place if its preceding message is deleted.
          setLines((old) => old.map((line) => {
            if (line.after === null || present.has(line.after)) return line
            const preceding = before.chat.slice(0, before.chat.findIndex((m) => m.id === line.after))
            return { ...line, after: preceding.reverse().find((m) => present.has(m.id))?.id ?? null }
          }))
        }
        if (connected && self && !changes.welcome) {
          for (const message of changes.messages) {
            const pings = message.type === 'text' ? pingCount(message.body, self.nickname) : 0
            if (pings) {
              play()
              for (let i = 1; i < pings; i++) {
                const timer = window.setTimeout(() => { timers.delete(timer); play() }, i * 50)
                timers.add(timer)
              }
            } else if (document.hidden && !preferences.peek().muteChatNotification) play()
          }
          if (preferences.peek().showLeaveJoinMsg && users !== before.users) {
            const next: TemporaryLine[] = []
            const line = (body: string) => next.push({ id: ++sequence, after: chat.at(-1)?.id ?? null, time: Date.now(), body })
            for (const user of changes.joined) line(`${user.nickname} joined`)
            for (const user of changes.left) line(`${user.nickname} left`)
            if (next.length) setLines((old) => [...old, ...next])
          }
        }
      })
      previous = next
    })
    return () => { off(); timers.forEach(window.clearTimeout) }
  }, [store])
  return lines
}
