import { useEffect, useRef, useState } from 'preact/hooks'
import { useRoute } from 'preact-iso'
import { preferences, settingsOpen } from '../app/state'
import { Controls } from '../components/room/Controls'
import { FilesWindow } from '../components/room/FilesWindow'
import { KickedScreen } from '../components/room/KickedScreen'
import { RoomContext, useRoomStore } from '../components/room/RoomContext'
import { Sidebar } from '../components/room/Sidebar'
import { RoomSettingsWindow } from '../components/room/admin/RoomSettingsWindow'
import { SettingsDialog } from '../components/settings/SettingsDialog'
import { UserCard } from '../components/room/UserCard'
import type { HoverName } from '../components/room/UserHoverName'
import { UserStrip } from '../components/room/UserStrip'
import { VideoArea } from '../components/room/VideoArea'
import { useRoomFullscreen } from '../components/room/useRoomFullscreen'
import { useRoomPresence } from '../components/room/useRoomPresence'
import { useRoomShortcuts } from '../components/room/useRoomShortcuts'
import { useUnreadChat } from '../components/room/useUnreadChat'
import { useRoom } from '../room/useRoom'
import styles from './RoomPage.module.css'

// RoomRoute owns the joined room for /room/<name>?access=<code>.
export function RoomRoute() {
  const { params, query } = useRoute()
  const store = useRoom(params.room, query.access)
  return <RoomContext.Provider value={store}><RoomPage key={`${params.room}:${query.access ?? ''}`} /></RoomContext.Provider>
}

export type RoomWindow = 'files' | 'settings'

export function RoomPage() {
  const store = useRoomStore()
  const page = useRef<HTMLDivElement>(null)
  const { fullscreen, idle, wake, toggle, error } = useRoomFullscreen(page)
  const [chatOpen, setChatOpen] = useState(true)
  const unreadChat = useUnreadChat(chatOpen)
  const [userlistHidden, setUserlistHidden] = useState(false)
  // Windows over the room: the desktop's files and the room's settings.
  const [roomWindow, setRoomWindow] = useState<RoomWindow | null>(null)
  const personalSettings = settingsOpen.value !== null
  const [hover, setHover] = useState<HoverName | null>(null)
  const wasConnected = useRef(false)
  const [screen, setScreen] = useState(store.neko.screen)
  useEffect(() => store.neko.on('screen', setScreen), [store])
  const connected = store.server.value === 'connected'
  if (connected) wasConnected.current = true
  // Narrow screens have no room beside the stream: the strip stays below it.
  const [narrow, setNarrow] = useState(() => window.matchMedia('(max-width: 780px)').matches)
  useEffect(() => {
    const query = window.matchMedia('(max-width: 780px)')
    const change = () => setNarrow(query.matches)
    query.addEventListener('change', change)
    return () => query.removeEventListener('change', change)
  }, [])
  const left = fullscreen || (preferences.value.userlistOnLeft && !narrow)
  const admin = store.rights.value.admin
  useRoomPresence()
  const files = store.rights.value.upload
  const { audioOnly, chatWidth } = preferences.value
  useEffect(() => {
    if ((!admin && roomWindow === 'settings') || (!files && roomWindow === 'files')) setRoomWindow(null)
  }, [admin, files, roomWindow])
  useEffect(() => { store.setAudioOnly(audioOnly) }, [store, audioOnly])
  useEffect(() => () => { settingsOpen.value = null }, [])
  useEffect(() => { setHover(null) }, [fullscreen, left, userlistHidden, idle])
  const overlay = personalSettings || roomWindow !== null
  useRoomShortcuts({
    windowOpen: overlay,
    onFullscreen: () => { void toggle() },
    onChat: () => setChatOpen((value) => !value),
    onUsers: () => setUserlistHidden((value) => !value),
    onMessage: () => {
      setChatOpen(true)
      requestAnimationFrame(() => page.current?.querySelector<HTMLTextAreaElement>('textarea[aria-label="Chat message"]')?.focus())
    },
  })
  const closeWindow = () => { setRoomWindow(null); wake() }
  if (store.kicked.value) return <>
    <KickedScreen />
    {personalSettings && <SettingsDialog />}
  </>
  return (
    <div ref={page} class={`${styles.page} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && idle && !overlay ? styles.idle : ''}`}
      data-chat-width={chatWidth} style={{ '--screen-ratio': screen.width > 0 && screen.height > 0 ? screen.width / screen.height : 16 / 9 }}
      onMouseMove={wake} onTouchStart={wake} onKeyDown={wake}
      // Icons, avatars and the stream are not things to drag around. The
      // sidebar keeps its text and pictures draggable.
      onDragStart={(e) => {
        const from = e.target instanceof Element ? e.target : (e.target as Node | null)?.parentElement
        if (!from?.closest('aside')) e.preventDefault()
      }}>
      {!userlistHidden && left && <UserStrip left fullscreen={fullscreen} hover={hover} onHover={setHover} />}
      <div class={styles.videoWrapper}>
        <VideoArea disconnected={!connected && wasConnected.current} error={error} fullscreen={fullscreen} />
        <div class={styles.toolbar}>
          <Controls fullscreen={fullscreen} userlistHidden={userlistHidden} chatOpen={chatOpen} unreadChat={unreadChat} window={roomWindow}
            onToggleUsers={() => setUserlistHidden((value) => !value)} onPersonalSettings={() => { settingsOpen.value = 'appearance' }}
            onChat={setChatOpen} onWindow={setRoomWindow} onFullscreen={() => { void toggle() }} />
          {!userlistHidden && !left && <UserStrip left={false} fullscreen={false} hover={hover} onHover={setHover} />}
        </div>
      </div>
      <Sidebar chatOpen={chatOpen} fullscreen={fullscreen} idle={idle && !overlay} />
      {personalSettings && <SettingsDialog onClose={wake} />}
      {roomWindow === 'files' && files && <FilesWindow onClose={closeWindow} />}
      {roomWindow === 'settings' && admin && <RoomSettingsWindow onClose={closeWindow} />}
      <UserCard hover={hover} />
    </div>
  )
}
