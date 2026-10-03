// State of one joined room, kept in sync with the server socket and the
// room's neko connection. Components read the signals and call the actions;
// they never talk to the sockets directly.

import { batch, computed, signal } from '@preact/signals'
import { parseStream } from '../components/room/admin/streamOptions'
import { NekoClient, type NekoStatus } from '../neko/client'
import type { ChatMessage, KickReason, Rights, RoomSettings, ServerMessage, User } from './protocol'
import { RoomSocket } from './socket'

// Reconnecting the stream backs off from 1.5 s to 30 s; after a few failed
// attempts in a row the user is told instead of seeing endless "connecting".
const NEKO_RETRY_MS = 1_500
const NEKO_RETRY_MAX_MS = 30_000
const NEKO_FAILURES_BEFORE_NOTICE = 3
const NEKO_FAILURE_NOTICE = "Can't connect to the room's video. Your network may be blocking it; still trying."
const TYPING_TIMEOUT_MS = 3_000

const noRights: Rights = { admin: false, trusted: false, remote: false, image: false, upload: false }

export interface DesktopUpload {
  state: 'idle' | 'uploading' | 'done' | 'error'
  progress: number // 0..1
  message: string
}

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
  readonly restartAvailable = signal(false)
  readonly restarting = signal<string | null>(null) // nickname

  // remote, as neko sees this tab
  readonly isHost = signal(false)

  // local playback: paused = no stream at all; audioOnly = sound without video
  readonly paused = signal(false)
  readonly desktopUpload = signal<DesktopUpload>({ state: 'idle', progress: 0, message: '' })
  readonly audioOnly = signal(false)

  readonly selfKey = computed(() => this.self.value?.key ?? null)
  readonly hasRemote = computed(() => this.isHost.value)

  private socket: RoomSocket
  private offs: (() => void)[] = []
  private nekoRetry?: number
  private uploadReset?: number
  private nekoRetryMs = NEKO_RETRY_MS
  private nekoFailures = 0
  private typingTimers = new Map<string, number>()

  constructor(
    readonly room: string,
    access?: string, // temporary access invite code
    { audioOnly = false } = {},
  ) {
    this.audioOnly.value = audioOnly
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
        if (s === 'connected') {
          this.restarting.value = null
          if (this.error.value === NEKO_FAILURE_NOTICE) this.error.value = null
          this.nekoRetryMs = NEKO_RETRY_MS
          this.nekoFailures = 0
        }
        if (s === 'disconnected') this.isHost.value = false
      }),
      neko.on('stream', (s) => (this.stream.value = s)),
      neko.on('host', () => (this.isHost.value = neko.isHost)),
      neko.on('closed', () => {
        if (this.paused.value) return
        // The token may be stale (neko restarted, session removed): ask for
        // a new one rather than retrying the old one.
        this.retryNekoToken()
      }),
    )
  }

  dispose() {
    window.clearTimeout(this.nekoRetry)
    window.clearTimeout(this.uploadReset)
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

  // Release the remote. With center (or the room's "always center" setting)
  // the pointer is moved to the middle of the screen first.
  dropRemote(center = false) {
    if (center || this.settings.value?.centerRemote) {
      const { width, height } = this.neko.screen
      this.neko.move(Math.round(width / 2), Math.round(height / 2))
    }
    this.neko.releaseControl()
  }

  // Stop receiving the stream entirely (the room stays joined).
  pause() {
    this.paused.value = true
    window.clearTimeout(this.nekoRetry)
    this.neko.disconnect()
  }

  resume() {
    if (!this.paused.value) return
    this.paused.value = false
    this.socket.send({ type: 'neko_token' })
  }

  // Switch between sound only and full video; reconnects the stream.
  setAudioOnly(audioOnly: boolean) {
    if (audioOnly === this.audioOnly.value) return
    this.audioOnly.value = audioOnly
    if (!this.paused.value) {
      this.neko.disconnect()
      this.socket.send({ type: 'neko_token' })
    }
  }

  // Admins: take the remote away from whoever holds it.
  resetRemote() {
    this.socket.send({ type: 'remote_reset' })
  }

  // Upload files into the room desktop's Downloads folder; progress and the
  // outcome are in desktopUpload.
  async uploadToDesktop(files: File[]) {
    if (files.length === 0 || this.desktopUpload.value.state === 'uploading') return
    window.clearTimeout(this.uploadReset)
    const set = (state: DesktopUpload['state'], progress: number, message: string) =>
      (this.desktopUpload.value = { state, progress, message })
    const what = files.length === 1 ? files[0].name : `${files.length} files`
    set('uploading', 0, `Uploading ${what}…`)
    try {
      if (!this.rights.value.upload) throw new Error('You are not allowed to upload files.')
      await this.neko.upload(files, (p) => set('uploading', p, `Uploading ${what}… ${Math.round(p * 100)}%`))
      set('done', 1, `Uploaded ${what} to Downloads.`)
    } catch (e) {
      set('error', 0, e instanceof Error ? e.message : 'Upload failed.')
    }
    this.uploadReset = window.setTimeout(() => set('idle', 0, ''), 5_000)
  }

  restart() {
    this.error.value = null
    this.socket.send({ type: 'restart' })
  }

  private retryNekoToken() {
    window.clearTimeout(this.nekoRetry)
    if (++this.nekoFailures >= NEKO_FAILURES_BEFORE_NOTICE && !this.restarting.value) {
      this.error.value = NEKO_FAILURE_NOTICE
    }
    const delay = this.nekoRetryMs
    this.nekoRetryMs = Math.min(this.nekoRetryMs * 2, NEKO_RETRY_MAX_MS)
    this.nekoRetry = window.setTimeout(() => {
      if (!this.paused.value && this.server.value === 'connected' && this.neko.status === 'disconnected') {
        this.socket.send({ type: 'neko_token' })
      }
    }, delay)
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
          this.restartAvailable.value = msg.restart
          this.error.value = null
        })
        break
      case 'neko':
        window.clearTimeout(this.nekoRetry)
        // The desktop answered; only the "can't connect" notice waits until
        // the stream actually connects.
        if (this.error.value !== NEKO_FAILURE_NOTICE) this.error.value = null
        if (!this.paused.value) {
          this.neko.connect(msg.path, msg.token, {
            audioOnly: this.audioOnly.value,
            video: this.settings.value?.stream,
          })
        }
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
      case 'room_settings': {
        const stream = this.settings.value?.stream
        this.settings.value = msg.settings
        // Follow the room's stream choice without reconnecting. A bigger
        // stream size needs a new media connection ("" = the full-size default).
        if (msg.settings.stream !== stream && msg.settings.stream && this.neko.status !== 'disconnected') {
          const scale = (id: string | undefined) => parseStream(id ?? '')?.scale ?? 100
          this.neko.setVideo(msg.settings.stream, { renew: scale(msg.settings.stream) > scale(stream) })
        }
        break
      }
      case 'remote':
        this.remoteHolder.value = msg.holder
        break
      case 'restarting':
        this.restarting.value = msg.by
        break
      case 'kicked':
        this.kicked.value = { reason: msg.reason, bannedUntil: msg.bannedUntil }
        this.neko.disconnect()
        break
      case 'error':
        this.error.value = msg.message
        // A restart can take longer than the first token retry. Keep trying
        // after a failed token request until the desktop comes back.
        if (this.restarting.value !== null && this.neko.status === 'disconnected' && !this.paused.value) {
          this.retryNekoToken()
        }
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
