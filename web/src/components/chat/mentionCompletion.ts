import type { User } from '../../room/protocol'
import { pingCount, pingName } from './parseMessage'

export interface MentionToken {
  start: number
  end: number
  query: string
}

export function activeMention(text: string, caret: number): MentionToken | null {
  if (caret < 0 || caret > text.length) return null
  const match = text.slice(0, caret).match(/(?:^|\s)@([^\s@]*)$/)
  if (!match) return null
  return {
    start: caret - match[1].length - 1,
    end: caret + (text.slice(caret).match(/^[^\s@]*/)?.[0].length ?? 0),
    query: match[1],
  }
}

export function mentionName(nickname: string): string {
  return nickname.replace(/\s/g, '')
}

export function mentionCandidates(users: ReadonlyMap<string, User>, selfKey: string | null, query: string, limit = 8): User[] {
  const search = query.toLowerCase()
  return [...users.values()].filter((user) => {
    const name = pingName(user.nickname)
    // Some nicknames contain @ or look like links; the parser cannot
    // mention them. Only offer completions that will actually highlight.
    return user.key !== selfKey && name.includes(search) && pingCount(`@${mentionName(user.nickname)} `, user.nickname) === 1
  }).sort((a, b) => {
    const first = pingName(a.nickname), second = pingName(b.nickname)
    return Number(second.startsWith(search)) - Number(first.startsWith(search))
      || first.localeCompare(second) || a.key.localeCompare(b.key)
  }).slice(0, Math.max(0, limit))
}

export function completeMention(text: string, token: MentionToken, nickname: string): { text: string; caret: number } {
  const replacement = `@${mentionName(nickname)} `
  return { text: text.slice(0, token.start) + replacement + text.slice(token.end), caret: token.start + replacement.length }
}
