// JSON API client. Errors carry the server's user-facing message.

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit = { method, headers: {} }
  if (body instanceof FormData) {
    init.body = body
  } else if (body !== undefined) {
    init.body = JSON.stringify(body)
    ;(init.headers as Record<string, string>)['Content-Type'] =
      'application/json'
  }

  let res: Response
  try {
    res = await fetch(path, init)
  } catch {
    throw new ApiError(0, 'Could not reach the server.')
  }
  if (res.status === 204) return undefined as T

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      data?.error ?? `Request failed (${res.status}).`,
    )
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// ---- shared response types (see docs/api.md) --------------------------------

export interface Me {
  username: string
  nickname: string
  nameColor: string
  avatarUrl: string
  admin: boolean
  verified: boolean
}

export interface ServerSettings {
  message: string
  registration: 'open' | 'invite'
  sourceUrl: string // where to get this server's source code (AGPL)
}

export interface RoomInfo {
  name: string
  access: 'public' | 'account' | 'verified' | 'invite'
  userCount: number
  open: boolean
}

export interface AdminUser extends Me {
  disabled: boolean
  createdAt: number
}

export interface Permission {
  room: string
  username: string
  remote: boolean
  image: boolean
  upload: boolean
  trusted: boolean
  invited: boolean
  banned: boolean
  inviteName: string
  bannedUntil: number | null
}

export interface InviteView {
  code: string
  room: string
  temporary: boolean
  name: string
  remote: boolean
  image: boolean
  upload: boolean
  uses: number
  maxUses: number | null
  expiresAt: number | null
  createdAt: number
  valid: boolean
  path: string
}
