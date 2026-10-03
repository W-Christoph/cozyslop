// State of one joined room, kept in sync with the server socket and the
// room's neko connection. Components read the signals and call the actions;
// they never talk to the sockets directly.

import { batch, computed, signal } from '@preact/signals'
import { NekoClient, type NekoStatus } from '../neko/client'
import type { ChatMessage, KickReason, Rights, RoomSettings, ServerMessage, User } from './protocol'
import { RoomSocket } from './socket'

const NEKO_RETRY_MS = 1_500
const TYPING_TIMEOUT_MS = 3_000

const noRights: Rights = { admin: false, trusted: false, remote: false, image: false, upload: false }

export interface Kick {
  reason: KickReason
  bannedUntil?: number | null // unix seconds; null = forever
}

export class RoomStore {
  readonly neko = new NekoClient()

  // connection
  readonly server = signal<'connecting' | 'connected'>('connecting')
  readonly video = signal<NekoStatus>('disconnected')
  readonly stream = signal<MediaStream | null>(null)
  readonly error = signal<string | null>(null)
  readonly kicked = signal<Kick | null>(null)

  // room
  readonly self = signal<User | null>(null)
  readonly rights = signal<Rights>(noRights)
  readonly settings = signal<RoomSettings | null>(null)
  readonly users = signal<ReadonlyMap<string, User>>(new Map())
  readonly chat = signal<readonly ChatMessage[]>([])
  readonly typing = signal<ReadonlySet<string>>(new Set()) // identity keys
  readonly remoteHolder = signal<string | null>(null) // identity key

  // remote, as neko sees this tab
  readonly isHost = signal(false)

  readonly selfKey = computed(() => this.self.value?.key ?? null)
  readonly hasRemote = computed(() => this.isHost.value)

  private socket: RoomSocket
  private offs: (() => void)[] = []
  private nekoRetry?: number
  private typingTimers = new Map<string, number>()

  constructor(
    readonly room: string,
    access?: string, // temporary access invite code
  ) {
    this.socket = new RoomSocket(room, access)
    const neko = this.neko
    this.offs.push(
      this.socket.on('open', () => (this.server.value = 'connected')),
      this.socket.on('close', () => {
        this.server.value = 'connecting'
        // The server drops our neko member with the socket; a new token
        // comes after reconnecting.
        neko.disconnect()
      }),
      this.socket.on('message', (msg) => this.onMessage(msg)),
      neko.on('status', (s) => {
        this.video.value = s
        if (s === 'disconnected') this.isHost.value = false
      }),
      neko.on('stream', (s) => (this.stream.value = s)),
      neko.on('host', () => (this.isHost.value = neko.isHost)),
      neko.on('closed', () => {
        // The token may be stale (neko restarted, session removed): ask for
        // a new one rather than retrying the old one.
        window.clearTimeout(this.nekoRetry)
        this.nekoRetry = window.setTimeout(() => this.socket.send({ type: 'neko_token' }), NEKO_RETRY_MS)
      }),
    )
  }

  dispose() {
    window.clearTimeout(this.nekoRetry)
    this.typingTimers.forEach((t) => window.clearTimeout(t))
    this.offs.forEach((off) => off())
    this.socket.close()
    this.neko.disconnect()
  }

  // ---- actions -------------------------------------------------------------

  sendChat(body: string) {
    this.socket.send({ type: 'chat_send', body })
  }

  editChat(id: number, body: string) {
    this.socket.send({ type: 'chat_edit', id, body })
  }

  deleteChat(id: number) {
    this.socket.send({ type: 'chat_delete', id })
  }

  setTyping(typing: boolean) {
    this.socket.send({ type: 'typing', typing })
  }

  setActive(active: boolean) {
    this.socket.send({ type: 'activity', active })
  }

  setMuted(muted: boolean) {
    this.socket.send({ type: 'muted', muted })
  }

  whisper(to: string, body: string) {
    this.socket.send({ type: 'whisper', to, body })
  }

  takeRemote() {
    this.neko.requestControl()
  }

  dropRemote() {
    this.neko.releaseControl()
  }

  // Admins: take the remote away from whoever holds it.
  resetRemote() {
    this.socket.send({ type: 'remote_reset' })
  }

  // ---- server messages -----------------------------------------------------

  private onMessage(msg: ServerMessage) {
    switch (msg.type) {
      case 'welcome':
        batch(() => {
          this.self.value = msg.self
          this.rights.value = msg.rights
          this.settings.value = msg.settings
          this.users.value = new Map(msg.users.map((u) => [u.key, u]))
          this.chat.value = msg.history
          this.remoteHolder.value = msg.remote
          this.error.value = null
        })
        break
      case 'neko':
        this.error.value = null
        this.neko.connect(msg.path, msg.token)
        break
      case 'user_joined':
      case 'user_updated':
        this.users.value = new Map(this.users.value).set(msg.user.key, msg.user)
        if (msg.user.key === this.selfKey.value) this.self.value = msg.user
        break
      case 'user_left': {
        const users = new Map(this.users.value)
        users.delete(msg.key)
        this.users.value = users
        this.setUserTyping(msg.key, false)
        break
      }
      case 'chat':
        this.chat.value = [...this.chat.value, msg.message]
        this.setUserTyping(msg.message.author, false)
        break
      case 'chat_edited':
        this.chat.value = this.chat.value.map((m) => (m.id === msg.id ? { ...m, body: msg.body, edited: true } : m))
        break
      case 'chat_deleted':
        this.chat.value = this.chat.value.filter((m) => m.id !== msg.id)
        break
      case 'typing':
        this.setUserTyping(msg.key, msg.typing)
        break
      case 'rights':
        this.rights.value = msg.rights
        break
      case 'room_settings':
        this.settings.value = msg.settings
        break
      case 'remote':
        this.remoteHolder.value = msg.holder
        break
      case 'kicked':
        this.kicked.value = { reason: msg.reason, bannedUntil: msg.bannedUntil }
        this.neko.disconnect()
        break
      case 'error':
        this.error.value = msg.message
        break
    }
  }

  private setUserTyping(key: string, typing: boolean) {
    window.clearTimeout(this.typingTimers.get(key))
    this.typingTimers.delete(key)
    const next = new Set(this.typing.value)
    if (typing) {
      next.add(key)
      // Drop the indicator if the "stopped typing" message never comes.
      this.typingTimers.set(key, window.setTimeout(() => this.setUserTyping(key, false), TYPING_TIMEOUT_MS))
    } else {
      next.delete(key)
    }
    this.typing.value = next
  }
}
