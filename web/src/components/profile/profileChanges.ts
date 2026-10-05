import type { Me } from '../../api'

function nameColor(color: string): string {
  const hex = color.replace('#', '').toLowerCase()
  return hex.length === 3 ? [...hex].map((c) => c + c).join('') : hex
}

export function profileChanged(user: Pick<Me, 'nickname' | 'nameColor'>, nickname: string, color: string, avatar: Blob | null): boolean {
  return nickname !== user.nickname || nameColor(color) !== nameColor(user.nameColor) || avatar !== null
}
