// Room WebSocket protocol. Mirrors server/internal/hub/protocol.go; keep both
// in sync.

export interface Rights {
  admin: boolean
  trusted: boolean
  remote: boolean
  image: boolean
  upload: boolean
}

export type Access = 'public' | 'account' | 'verified' | 'invite'

export interface RoomSettings {
  name: string
  access: Access
  hidden: boolean
  remoteOwnership: boolean
  centerRemote: boolean
  defaultRemote: boolean
  defaultImage: boolean
  defaultUpload: boolean
  screen: string // "1280x720@30"; "" = the room container's default
  stream: string // capture pipeline viewers watch, e.g. "b2500-s100"; "" = neko's default
}

export interface User {
  key: string // identity key, "u:<id>" or "a:<anon id>"
  username: string // account name; "" for anonymous users
  nickname: string
  nameColor: string
  avatarUrl: string
  anonymous: boolean
  admin: boolean
  active: boolean
  muted: boolean
  joinedAt: number
  lastSeen: number // unix ms
}

export interface ChatMessage {
  id: number // negative for whispers (not stored)
  author: string // identity key
  nickname: string
  nameColor: string
  anonymous: boolean
  type: 'text' | 'image' | 'video' | 'whisper'
  body: string
  mediaUrl?: string
  edited: boolean
  time: number // unix ms
}

export type KickReason = 'banned' | 'account' | 'verified' | 'invite' | 'kicked' | 'deleted'

export type ServerMessage =
  | {
      type: 'welcome'
      clientId: string
      self: User
      rights: Rights
      settings: RoomSettings
      users: User[]
      history: ChatMessage[] // oldest first; replaces what we had
      remote: string | null
      restart: boolean
    }
  | { type: 'neko'; token: string; path: string }
  | { type: 'user_joined' | 'user_updated'; user: User }
  | { type: 'user_left'; key: string }
  | { type: 'chat'; message: ChatMessage }
  | { type: 'chat_edited'; id: number; body: string }
  | { type: 'chat_deleted'; id: number }
  | { type: 'typing'; key: string; typing: boolean }
  | { type: 'rights'; rights: Rights }
  | { type: 'room_settings'; settings: RoomSettings }
  | { type: 'remote'; holder: string | null }
  | { type: 'restarting'; by: string }
  | { type: 'kicked'; reason: KickReason; bannedUntil?: number | null }
  | { type: 'error'; message: string }

export type ClientMessage =
  | { type: 'chat_send'; body: string }
  | { type: 'chat_edit'; id: number; body: string }
  | { type: 'chat_delete'; id: number }
  | { type: 'typing'; typing: boolean }
  | { type: 'activity'; active: boolean }
  | { type: 'muted'; muted: boolean }
  | { type: 'whisper'; to: string; body: string }
  | { type: 'remote_reset' }
  | { type: 'restart' }
  | { type: 'neko_token' }
