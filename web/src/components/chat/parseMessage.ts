import { find } from 'linkifyjs'
import type { ChatMessage, User } from '../../room/protocol'

export type MessagePart =
  | { type: 'text'; text: string }
  | { type: 'ping'; text: string; target: string }
  | { type: 'url'; text: string; href: string }

export function pingName(nickname: string): string {
  return nickname.replace(/\s/g, '').toLowerCase()
}

// Linkify first: @ inside a URL or email must not become a mention.
export function parseMessage(body: string): MessagePart[] {
  const parts: MessagePart[] = []
  function text(value: string) {
    for (const match of value.matchAll(/(@[^ \n@]*)|([^@]+)/g)) {
      const value = match[0]
      parts.push(value.startsWith('@') && value.length > 1
        ? { type: 'ping', text: value, target: value.slice(1).toLowerCase() }
        : { type: 'text', text: value })
    }
  }
  let offset = 0
  for (const link of find(body)) {
    text(body.slice(offset, link.start))
    parts.push(link.type === 'url'
      ? { type: 'url', text: link.value, href: link.href }
      : { type: 'text', text: link.value })
    offset = link.end
  }
  text(body.slice(offset))
  return parts
}

export function pingCount(body: string, nickname: string): number {
  const target = pingName(nickname)
  return target ? parseMessage(body).filter((p) => p.type === 'ping' && p.target === target).length : 0
}

export function matchedPingNames(users: ReadonlyMap<string, User>): ReadonlySet<string> {
  return new Set([...users.values()].map((user) => pingName(user.nickname)))
}

export function groupMessages(messages: readonly ChatMessage[]): ChatMessage[][] {
  const groups: ChatMessage[][] = []
  for (const message of messages) {
    const last = groups.at(-1)
    if (last?.[0].author === message.author) last.push(message)
    else groups.push([message])
  }
  return groups
}

const clock = new Intl.DateTimeFormat('en-US', { hour: 'numeric', minute: '2-digit', hour12: true })
export function messageTime(time: number): string {
  return clock.format(time)
}
