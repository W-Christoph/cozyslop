// Connection to the CozyCast server for one room. Reconnects automatically.

import { Emitter } from '../neko/emitter'

export interface Permissions {
  remote: boolean
  upload: boolean
}

export type ServerMessage =
  | { type: 'welcome'; id: string; name: string; permissions: Permissions }
  | { type: 'neko'; token: string; path: string }
  | { type: 'error'; message: string }

export type ClientMessage = { type: 'neko/token' }

interface RoomSocketEvents {
  open: () => void
  close: () => void
  message: (msg: ServerMessage) => void
}

const RETRY_MIN_MS = 1_000
const RETRY_MAX_MS = 15_000

export class RoomSocket extends Emitter<RoomSocketEvents> {
  private ws?: WebSocket
  private retryMs = RETRY_MIN_MS
  private retryTimer?: number
  private closed = false

  constructor(
    private readonly room: string,
    private readonly name: string,
  ) {
    super()
    this.open()
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
    const url = `${proto}//${location.host}/api/rooms/${encodeURIComponent(this.room)}/ws?name=${encodeURIComponent(this.name)}`
    const ws = new WebSocket(url)
    this.ws = ws

    ws.onopen = () => {
      this.retryMs = RETRY_MIN_MS
      this.emit('open')
    }
    ws.onmessage = (e) => this.emit('message', JSON.parse(e.data) as ServerMessage)
    ws.onclose = () => {
      this.emit('close')
      if (this.closed) return
      this.retryTimer = window.setTimeout(() => this.open(), this.retryMs)
      this.retryMs = Math.min(this.retryMs * 2, RETRY_MAX_MS)
    }
  }
}
