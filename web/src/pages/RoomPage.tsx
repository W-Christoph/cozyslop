import { useEffect, useRef, useState } from 'preact/hooks'
import { useRoute } from 'preact-iso'
import { preferences, settingsOpen } from '../app/state'
import { Controls } from '../components/room/Controls'
import { FilesWindow } from '../components/room/FilesWindow'
import { KickedScreen } from '../components/room/KickedScreen'
import { RoomContext, useRoomStore } from '../components/room/RoomContext'
import { Sidebar, type SidebarTab } from '../components/room/Sidebar'
import { RoomSettingsWindow } from '../components/room/admin/RoomSettingsWindow'
import { SettingsDialog } from '../components/settings/SettingsDialog'
import { UserHoverName, type HoverName } from '../components/room/UserHoverName'
import { UserStrip } from '../components/room/UserStrip'
import { VideoArea } from '../components/room/VideoArea'
import { useRoomFullscreen } from '../components/room/useRoomFullscreen'
import { useRoomPresence } from '../components/room/useRoomPresence'
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
  const [sidebar, setSidebar] = useState<SidebarTab>('CHAT')
  const [userlistHidden, setUserlistHidden] = useState(false)
  // Windows over the room: the desktop's files and the room's settings.
  const [roomWindow, setRoomWindow] = useState<RoomWindow | null>(null)
  const personalSettings = settingsOpen.value !== null
  const [hover, setHover] = useState<HoverName | null>(null)
  const wasConnected = useRef(false)
  const connected = store.server.value === 'connected'
  if (connected) wasConnected.current = true
  const left = fullscreen || preferences.value.userlistOnLeft
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
  const closeWindow = () => { setRoomWindow(null); wake() }
  if (store.kicked.value) return <>
    <KickedScreen />
    {personalSettings && <SettingsDialog />}
  </>
  return (
    <div ref={page} class={`${styles.page} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && idle && !overlay ? styles.idle : ''}`}
      data-chat-width={chatWidth}
      onMouseMove={wake} onTouchStart={wake} onKeyDown={wake}
      // Icons, avatars and the stream are not things to drag around. The
      // sidebar keeps its text and pictures draggable.
      onDragStart={(e) => {
        const from = e.target instanceof Element ? e.target : (e.target as Node | null)?.parentElement
        if (!from?.closest('aside')) e.preventDefault()
      }}>
      {!userlistHidden && left && <UserStrip left fullscreen={fullscreen} onHover={setHover} />}
      <div class={styles.videoWrapper}>
        <VideoArea />
        <div class={styles.toolbar}>
          <Controls fullscreen={fullscreen} userlistHidden={userlistHidden} sidebar={sidebar} window={roomWindow}
            onToggleUsers={() => setUserlistHidden((value) => !value)} onPersonalSettings={() => { settingsOpen.value = 'appearance' }}
            onSidebar={setSidebar} onWindow={setRoomWindow} onFullscreen={() => { void toggle() }} />
          {!userlistHidden && !left && <UserStrip left={false} fullscreen={false} onHover={setHover} />}
        </div>
        {error && <div role="alert" class={styles.error}>{error}</div>}
      </div>
      <Sidebar tab={sidebar} fullscreen={fullscreen} idle={idle && !overlay} />
      {personalSettings && <SettingsDialog onClose={wake} />}
      {roomWindow === 'files' && files && <FilesWindow onClose={closeWindow} />}
      {roomWindow === 'settings' && admin && <RoomSettingsWindow onClose={closeWindow} />}
      {!connected && wasConnected.current && <div class={styles.disconnected} role="status">DISCONNECTED{store.error.value && <span>{store.error.value}</span>}</div>}
      <UserHoverName hover={hover} />
      {!userlistHidden && !fullscreen && <a class={`${styles.copyright} ${left ? styles.leftCopyright : ''}`} href="/license" target="_blank" rel="noopener">Copyright (C) 2024 Vorlent</a>}
    </div>
  )
}
