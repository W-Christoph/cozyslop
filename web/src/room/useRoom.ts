// Joins a room: keeps the server socket and the neko connection in sync and
// exposes their state as signals.

import { signal } from '@preact/signals'
import { useEffect, useMemo } from 'preact/hooks'
import { NekoClient, type NekoStatus } from '../neko/client'
import { RoomSocket, type Permissions } from './socket'

const NEKO_RETRY_MS = 1_500

export function useRoom(room: string, name: string) {
  const state = useMemo(
    () => ({
      neko: new NekoClient(),
      server: signal<'connecting' | 'connected'>('connecting'),
      video: signal<NekoStatus>('disconnected'),
      stream: signal<MediaStream | null>(null),
      permissions: signal<Permissions>({ remote: false, upload: false }),
      isHost: signal(false),
      canHost: signal(false),
      hasHost: signal(false),
      error: signal<string | null>(null),
    }),
    [room, name],
  )

  useEffect(() => {
    const { neko } = state
    const socket = new RoomSocket(room, name)
    let retry: number | undefined

    const offs = [
      socket.on('open', () => (state.server.value = 'connected')),
      socket.on('close', () => {
        state.server.value = 'connecting'
        // Our server deletes the neko member when this socket drops; a new
        // token arrives after reconnecting.
        neko.disconnect()
      }),
      socket.on('message', (msg) => {
        switch (msg.type) {
          case 'welcome':
            state.permissions.value = msg.permissions
            break
          case 'neko':
            state.error.value = null
            neko.connect(msg.path, msg.token)
            break
          case 'error':
            state.error.value = msg.message
            break
        }
      }),
      neko.on('status', (s) => {
        state.video.value = s
        if (s === 'disconnected') state.isHost.value = state.hasHost.value = false
      }),
      neko.on('stream', (s) => (state.stream.value = s)),
      neko.on('canHost', (v) => (state.canHost.value = v)),
      neko.on('host', (hostId) => {
        state.hasHost.value = hostId !== undefined
        state.isHost.value = neko.isHost
      }),
      neko.on('closed', () => {
        // Token may be stale (neko restarted, session removed): ask for a
        // new one rather than retrying the old one.
        window.clearTimeout(retry)
        retry = window.setTimeout(() => socket.send({ type: 'neko/token' }), NEKO_RETRY_MS)
      }),
    ]

    return () => {
      window.clearTimeout(retry)
      offs.forEach((off) => off())
      socket.close()
      neko.disconnect()
    }
  }, [state])

  return state
}
