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
  defaultRemote: boolean
  defaultImage: boolean
  defaultUpload: boolean
  screen: string // "1280x720@30"; "" = the room container's default
  stream: string // capture pipeline viewers watch, e.g. "b2500-s100-veryfast"; "" = neko's default
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
  muted: boolean // not listening to the stream
  paused: boolean // not listening because the stream is paused; implies muted
  listening: boolean // the sound without the picture; never with muted
  joinedAt: number
}

export interface ChatMessage {
  id: number // negative for whispers (not stored)
  author: string // identity key
  nickname: string
  nameColor: string
  avatarUrl?: string // available even when the author has left the room
  anonymous: boolean
  type: 'text' | 'image' | 'video' | 'whisper'
  body: string
  mediaUrl?: string
  edited: boolean
  deleted?: boolean // set locally on chat_deleted; history never contains deleted messages
  time: number // unix ms
}

export type KickReason = 'banned' | 'account' | 'verified' | 'invite' | 'kicked' | 'deleted' | 'not_found' | 'session' | 'room_changed'

export type DesktopState = 'online' | 'offline'

// The watchlog: the episode VLC or Firefox plays on the desktop, and the
// suggestion to mark one watched once it ended (server/internal/hub/watch.go).
export interface WatchPlaying {
  animeId: number // 0: the show was not recognised
  title: string
  episode: number
}

export interface WatchViewer {
  key: string // identity key
  nickname: string
  coverage: number // share of the episode they saw, 0 to 1
  checked: boolean // gets the episode on save
}

export interface WatchProposal {
  id: number
  animeId: number // 0: no show; one must be picked before saving
  title: string
  coverUrl: string
  episodes: number | null
  episode: number
  file: string
  applyAt: number // unix ms it is saved by itself; 0 while there is no show or the episode is past its last
  viewers: WatchViewer[]
}

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
      desktop: DesktopState
      windowTitle: string // of the window in front on the desktop; "" if unknown
      playing?: WatchPlaying | null
      watch?: WatchProposal | null
    }
  | { type: 'neko'; token: string; path: string }
  | { type: 'neko_unavailable'; message: string }
  // The desktop went offline (after a few seconds without it) or came back.
  | { type: 'desktop'; state: DesktopState }
  | { type: 'user_joined' | 'user_updated'; user: User }
  | { type: 'user_left'; key: string }
  | { type: 'chat'; message: ChatMessage }
  | { type: 'chat_edited'; id: number; body: string }
  | { type: 'chat_deleted'; id: number }
  | { type: 'typing'; key: string; typing: boolean }
  | { type: 'rights'; rights: Rights }
  | { type: 'room_settings'; settings: RoomSettings }
  | { type: 'remote'; holder: string | null }
  | { type: 'window_title'; title: string }
  | { type: 'playing'; playing: WatchPlaying | null }
  | { type: 'watch'; proposal: WatchProposal | null } // null: saved or dismissed
  // A small still of the desktop, answering preview; image "" when there is none.
  | { type: 'preview'; image: string; time?: number }
  | { type: 'restarting'; by: string }
  | { type: 'file_result'; action: 'delete' | 'play'; name: string; error: string } // error "" if it worked
  | { type: 'kicked'; reason: KickReason; bannedUntil?: number | null }
  | { type: 'error'; message: string }

export type ClientMessage =
  | { type: 'chat_send'; body: string }
  | { type: 'chat_edit'; id: number; body: string }
  | { type: 'chat_delete'; id: number }
  | { type: 'typing'; typing: boolean }
  | { type: 'activity'; active: boolean }
  | { type: 'muted'; muted: boolean; paused: boolean; listening: boolean }
  | { type: 'whisper'; to: string; body: string }
  | { type: 'remote_reset' }
  | { type: 'restart' }
  | { type: 'neko_token' }
  | { type: 'preview' } // a still of the desktop, for the chat view while paused
  | { type: 'file_delete' | 'file_play'; name: string } // a file in the desktop's Downloads
  | { type: 'watch_check'; id: number; key: string; checked: boolean }
  | { type: 'watch_episode'; id: number; episode: number }
  | { type: 'watch_anime'; id: number; animeId: number } // AniList id
  | { type: 'watch_save' | 'watch_dismiss'; id: number }
