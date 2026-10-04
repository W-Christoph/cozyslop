// Minimal client for the neko v3 protocol: WebSocket signalling, one WebRTC
// peer for audio/video, and a data channel for low-latency input.
// Based on demodesk/neko-client (Apache-2.0), without Vue or a state store.

import { Emitter } from './emitter'

export type NekoStatus = 'connecting' | 'connected' | 'disconnected'

export interface ScreenSize {
  width: number
  height: number
  rate: number
}

export interface NekoEvents {
  status: (status: NekoStatus) => void
  // Fired when the connection ends without disconnect() being called.
  // reason is set when neko said why (e.g. the session was removed).
  closed: (reason?: string) => void
  stream: (stream: MediaStream) => void
  host: (hostId: string | undefined) => void
  screen: (size: ScreenSize) => void
  canHost: (canHost: boolean) => void
  // The desktop's clipboard changed; neko only tells the host.
  clipboard: (text: string) => void
}

// Binary input opcodes (neko server/internal/webrtc/payload).
const OP_MOVE = 0x01
const OP_SCROLL = 0x02
const OP_KEY_DOWN = 0x03
const OP_KEY_UP = 0x04
const OP_BTN_DOWN = 0x05
const OP_BTN_UP = 0x06

// neko sends a heartbeat every 10s by default.
const STALE_TIMEOUT_MS = 25_000

interface Signal {
  sdp: string
  iceservers?: RTCIceServer[]
}

export class NekoClient extends Emitter<NekoEvents> {
  sessionId = ''
  hostId: string | undefined
  canHost = false
  screen: ScreenSize = { width: 1280, height: 720, rate: 30 }
  status: NekoStatus = 'disconnected'

  private ws?: WebSocket
  private path = ''
  private token = ''
  private audioOnly = false
  private video = ''
  private peer?: RTCPeerConnection
  private channel?: RTCDataChannel
  private pendingCandidates: RTCIceCandidateInit[] = []
  private lastMessage = 0
  private staleTimer?: number
  private heldButtons = new Set<number>()

  get isHost() {
    return this.sessionId !== '' && this.hostId === this.sessionId
  }

  // path is the room's neko prefix on our server, e.g. /neko/default.
  // video picks a capture pipeline (quality preset); with audioOnly, neko
  // sends no video track.
  connect(path: string, token: string, { audioOnly = false, video = '' } = {}) {
    this.teardown()
    this.setStatus('connecting')
    this.path = path
    this.token = token
    this.audioOnly = audioOnly
    this.video = video

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const ws = new WebSocket(`${proto}//${location.host}${path}/api/ws?token=${encodeURIComponent(token)}`)
    this.ws = ws

    ws.onopen = () => {
      this.lastMessage = Date.now()
      this.staleTimer = window.setInterval(() => {
        if (Date.now() - this.lastMessage > STALE_TIMEOUT_MS) this.fail('connection timed out')
      }, 5_000)
      this.requestPeer()
    }
    ws.onmessage = (e) => {
      this.lastMessage = Date.now()
      const { event, payload } = JSON.parse(e.data)
      void this.onMessage(event, payload)
    }
    ws.onclose = () => this.fail()
  }

  disconnect() {
    this.teardown()
    this.setStatus('disconnected')
  }

  // ---- control ---------------------------------------------------------

  requestControl() {
    this.send('control/request')
  }

  releaseControl() {
    this.releaseButtons()
    this.send('control/release')
  }

  move(x: number, y: number) {
    this.sendInput(OP_MOVE, 4, (v) => {
      v.setUint16(3, x)
      v.setUint16(5, y)
    })
  }

  scroll(deltaX: number, deltaY: number, controlKey = false) {
    this.sendInput(OP_SCROLL, 5, (v) => {
      v.setInt16(3, deltaX)
      v.setInt16(5, deltaY)
      v.setUint8(7, controlKey ? 1 : 0)
    })
  }

  // button: 1 = left, 2 = middle, 3 = right (X11 numbering)
  buttonDown(button: number) {
    this.heldButtons.add(button)
    this.sendInput(OP_BTN_DOWN, 4, (v) => v.setUint32(3, button))
  }

  buttonUp(button: number) {
    if (!this.heldButtons.delete(button)) return
    this.sendInput(OP_BTN_UP, 4, (v) => v.setUint32(3, button))
  }

  releaseButtons() {
    this.heldButtons.forEach((button) => this.buttonUp(button))
  }

  keyDown(keysym: number) {
    this.sendInput(OP_KEY_DOWN, 4, (v) => v.setUint32(3, keysym))
  }

  keyUp(keysym: number) {
    this.sendInput(OP_KEY_UP, 4, (v) => v.setUint32(3, keysym))
  }

  // Switch the running stream to another capture pipeline. renew starts a
  // new peer connection for it; needed when the new stream's frames are
  // bigger (see renewPeer).
  setVideo(id: string, { renew = false } = {}) {
    this.video = id
    if (renew && this.peer && !this.audioOnly) this.renewPeer()
    else this.send('signal/video', { selector: { type: 'exact', id } })
  }

  paste(text: string) {
    this.send('control/paste', { text })
  }

  // Upload files into the desktop's Downloads folder (neko's file transfer;
  // neko checks the upload right). Resolves when the upload finished.
  upload(files: File[], onProgress?: (fraction: number) => void): Promise<void> {
    if (!this.token) return Promise.reject(new Error('Not connected to the room.'))
    const form = new FormData()
    for (const f of files) form.append('files', f, f.name)
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest()
      xhr.open('POST', `${this.path}/api/filetransfer?token=${encodeURIComponent(this.token)}`)
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress?.(e.loaded / e.total)
      }
      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) resolve()
        else if (xhr.status === 403) reject(new Error('You are not allowed to upload files.'))
        else reject(new Error(`Upload failed (${xhr.status}).`))
      }
      xhr.onerror = () => reject(new Error('Upload failed: connection lost.'))
      xhr.send(form)
    })
  }

  // ---- internals -------------------------------------------------------

  private async onMessage(event: string, payload: any) {
    switch (event) {
      case 'system/init':
        this.sessionId = payload.session_id
        this.updateProfile(payload.sessions?.[payload.session_id]?.profile)
        this.updateScreen(payload.screen_size)
        this.updateHost(payload.control_host)
        break
      case 'system/disconnect':
        this.fail(payload?.message ?? 'disconnected by server')
        break
      case 'signal/provide':
        await this.createPeer(payload as Signal)
        break
      case 'signal/offer':
        await this.answer((payload as Signal).sdp)
        break
      case 'signal/candidate':
        if (this.peer?.remoteDescription) await this.peer.addIceCandidate(payload)
        else this.pendingCandidates.push(payload)
        break
      case 'control/host':
        this.updateHost(payload)
        break
      case 'screen/updated': {
        const before = this.screen
        this.updateScreen(payload)
        if (this.screen.width > before.width || this.screen.height > before.height) this.renewPeer()
        break
      }
      case 'session/profile':
        if (payload.id === this.sessionId) this.updateProfile(payload)
        break
      case 'clipboard/updated':
        if (typeof payload?.text === 'string') this.emit('clipboard', payload.text)
        break
    }
  }

  // neko answers with signal/provide; asking again on the same session
  // replaces the peer connection.
  private requestPeer() {
    const selector = this.video ? { type: 'exact', id: this.video } : undefined
    this.send('signal/request', { video: { disabled: this.audioOnly, selector }, audio: {} })
  }

  // Swap the media connection for a new one; the WebSocket session, and with
  // it the remote, stays. Done whenever the frame size grows mid-stream:
  // Chrome's hardware H.264 decoder then shows garbage (the picture tiled
  // and squashed) until it is replaced, and a new peer gets a new decoder.
  private renewPeer() {
    if (!this.peer || this.audioOnly) return
    this.closePeer()
    this.requestPeer()
  }

  private closePeer() {
    if (this.peer) {
      this.peer.onicecandidate = this.peer.onconnectionstatechange = null
      this.peer.ontrack = this.peer.ondatachannel = null
      this.peer.close()
      this.peer = undefined
    }
    this.channel = undefined
    this.pendingCandidates = []
  }

  private async createPeer({ sdp, iceservers }: Signal) {
    const peer = new RTCPeerConnection({ iceServers: iceservers ?? [] })
    this.peer = peer

    peer.onicecandidate = (e) => {
      if (e.candidate) this.send('signal/candidate', e.candidate.toJSON())
    }
    peer.onconnectionstatechange = () => {
      if (peer !== this.peer) return
      switch (peer.connectionState) {
        case 'connected':
          this.setStatus('connected')
          break
        case 'failed':
        case 'closed':
          this.fail('media connection failed')
          break
      }
    }
    peer.ontrack = (e) => {
      if (e.streams[0]) this.emit('stream', e.streams[0])
    }
    peer.ondatachannel = (e) => {
      this.channel = e.channel
      this.channel.binaryType = 'arraybuffer'
    }

    await this.answer(sdp)
  }

  private async answer(sdp: string) {
    const peer = this.peer
    if (!peer) return
    await peer.setRemoteDescription({ type: 'offer', sdp })
    for (const c of this.pendingCandidates.splice(0)) {
      await peer.addIceCandidate(c).catch(() => {})
    }
    const answer = await peer.createAnswer()
    // Ask Chromium for stereo Opus.
    answer.sdp = answer.sdp?.replace(/(stereo=1;)?useinbandfec=1/, 'useinbandfec=1;stereo=1')
    await peer.setLocalDescription(answer)
    this.send('signal/answer', { sdp: answer.sdp })
  }

  private updateHost(host: { has_host: boolean; host_id?: string } | undefined) {
    if (this.isHost && (!host?.has_host || host.host_id !== this.sessionId)) this.releaseButtons()
    this.hostId = host?.has_host ? host.host_id : undefined
    this.emit('host', this.hostId)
  }

  private updateScreen(size: ScreenSize | undefined) {
    if (!size) return
    this.screen = size
    this.emit('screen', size)
  }

  private updateProfile(profile: { can_host?: boolean } | undefined) {
    this.canHost = !!profile?.can_host
    this.emit('canHost', this.canHost)
  }

  private send(event: string, payload?: unknown) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ event, payload }))
    }
  }

  private sendInput(op: number, length: number, write: (v: DataView) => void) {
    if (this.channel?.readyState !== 'open') return
    const buf = new ArrayBuffer(3 + length)
    const view = new DataView(buf)
    view.setUint8(0, op)
    view.setUint16(1, length)
    write(view)
    this.channel.send(buf)
  }

  private setStatus(status: NekoStatus) {
    if (this.status === status) return
    this.status = status
    this.emit('status', status)
  }

  private fail(reason?: string) {
    if (!this.ws) return // already torn down
    this.teardown()
    this.setStatus('disconnected')
    this.emit('closed', reason)
  }

  private teardown() {
    this.releaseButtons()
    window.clearInterval(this.staleTimer)
    if (this.ws) {
      this.ws.onopen = this.ws.onmessage = this.ws.onclose = null
      this.ws.close()
      this.ws = undefined
    }
    this.closePeer()
    this.sessionId = ''
    this.hostId = undefined
  }
}
