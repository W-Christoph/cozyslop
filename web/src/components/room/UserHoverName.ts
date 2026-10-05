import type { User } from '../../room/protocol'

export function userIdentity(user: Pick<User, 'key' | 'anonymous' | 'username'> | string): string {
  const key = typeof user === 'string' ? user : user.key
  const anonymous = typeof user === 'string' ? key.startsWith('a:') : user.anonymous
  return anonymous ? `Anon(${key.replace(/^a:/, '').slice(0, 4)})` : typeof user === 'string' ? key : user.username
}

export interface HoverName { user: User; x: number; y: number; left: boolean }
