// Connection to the CozyCast server for one room. Reconnects automatically.

import { Emitter } from '../neko/emitter'
import type { ClientMessage, ServerMessage } from './protocol'

interface RoomSocketEvents {
  open: () => void
  close: () => void
  message: (msg: ServerMessage) => void
}

const RETRY_MIN_MS = 1_000
const RETRY_MAX_MS = 15_000
// The server closes with this code when it ended the session on purpose
// (kicked, banned, not allowed in); reconnecting would not help.
const CLOSE_KICKED = 4000

export class RoomSocket extends Emitter<RoomSocketEvents> {
  private ws?: WebSocket
  private retryMs = RETRY_MIN_MS
  private retryTimer?: number
  private closed = false

  constructor(
    private readonly room: string,
    private readonly access?: string, // temporary access invite code
    autoConnect = true,
  ) {
    super()
    if (autoConnect) this.connect()
  }

  connect() {
    if (!this.closed && !this.ws) this.open()
  }

  send(msg: ClientMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(msg))
  }

  close() {
    this.closed = true
    window.clearTimeout(this.retryTimer)
    this.ws?.close()
  }

  private open() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const query = this.access ? `?access=${encodeURIComponent(this.access)}` : ''
    const url = `${proto}//${location.host}/api/rooms/${encodeURIComponent(this.room)}/ws${query}`
    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      this.retryMs = RETRY_MIN_MS
      this.emit('open')
    }
    ws.onmessage = (e) => this.emit('message', JSON.parse(e.data) as ServerMessage)
    ws.onclose = (e) => {
      this.emit('close')
      if (this.closed || e.code === CLOSE_KICKED) return
      this.retryTimer = window.setTimeout(() => this.open(), this.retryMs)
      this.retryMs = Math.min(this.retryMs * 2, RETRY_MAX_MS)
    }
  }
}
