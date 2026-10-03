import type { ChatMessage, User } from '../../room/protocol'

export interface ChatSnapshot {
  chat: readonly ChatMessage[]
  users: ReadonlyMap<string, User>
  self: User | null
  connected: boolean
}

// The store replaces self and history together only for a welcome snapshot.
// Compare references as well as IDs: reconnect history may contain unseen IDs.
export function chatChanges(previous: ChatSnapshot, next: ChatSnapshot) {
  const welcome = !previous.self || (next.self !== previous.self && next.chat !== previous.chat)
  const known = new Set(previous.chat.map((m) => m.id))
  const live = next.connected && next.self !== null && !welcome
  return {
    welcome,
    messages: live ? next.chat.filter((m) => !known.has(m.id) && m.author !== next.self?.key) : [],
    joined: live ? [...next.users.values()].filter((u) => !u.anonymous && !previous.users.has(u.key)) : [],
    left: live ? [...previous.users.values()].filter((u) => !u.anonymous && !next.users.has(u.key)) : [],
  }
}
