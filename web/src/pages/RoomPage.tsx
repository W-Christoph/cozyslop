import { useEffect, useRef, useState } from 'preact/hooks'
import { useRoute } from 'preact-iso'
import { preferences } from '../app/state'
import { Controls } from '../components/room/Controls'
import { KickedScreen } from '../components/room/KickedScreen'
import { PersonalSettings } from '../components/room/PersonalSettings'
import { RoomContext, useRoomStore } from '../components/room/RoomContext'
import { Sidebar, type SidebarTab } from '../components/room/Sidebar'
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

export function RoomPage() {
  const store = useRoomStore()
  const page = useRef<HTMLDivElement>(null)
  const { fullscreen, idle, wake, toggle, error } = useRoomFullscreen(page)
  const [sidebar, setSidebar] = useState<SidebarTab>('CHAT')
  const [userlistHidden, setUserlistHidden] = useState(false)
  const [personalSettings, setPersonalSettings] = useState(false)
  const [hover, setHover] = useState<HoverName | null>(null)
  const wasConnected = useRef(false)
  const connected = store.server.value === 'connected'
  if (connected) wasConnected.current = true
  const left = fullscreen || preferences.value.userlistOnLeft
  const admin = store.rights.value.admin
  useRoomPresence()
  const files = store.rights.value.upload
  useEffect(() => {
    if ((!admin && sidebar === 'SETTINGS') || (!files && sidebar === 'FILES')) setSidebar('NOTHING')
  }, [admin, files, sidebar])
  useEffect(() => { setHover(null) }, [fullscreen, left, userlistHidden, idle])
  if (store.kicked.value) return <KickedScreen />
  return (
    <div ref={page} class={`${styles.page} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && idle && !personalSettings ? styles.idle : ''}`}
      onMouseMove={wake} onTouchStart={wake} onKeyDown={wake}>
      {!userlistHidden && left && <UserStrip left fullscreen={fullscreen} onHover={setHover} />}
      <div class={styles.videoWrapper}>
        <VideoArea />
        <div class={styles.toolbar}>
          <Controls fullscreen={fullscreen} userlistHidden={userlistHidden} sidebar={sidebar}
            onToggleUsers={() => setUserlistHidden((value) => !value)} onPersonalSettings={() => setPersonalSettings(true)}
            onSidebar={setSidebar} onFullscreen={() => { void toggle() }} />
          {!userlistHidden && !left && <UserStrip left={false} fullscreen={false} onHover={setHover} />}
        </div>
        {error && <div role="alert" class={styles.error}>{error}</div>}
      </div>
      <Sidebar tab={sidebar} fullscreen={fullscreen} idle={idle && !personalSettings} />
      {personalSettings && <PersonalSettings onClose={() => { setPersonalSettings(false); wake() }} />}
      {!connected && wasConnected.current && <div class={styles.disconnected} role="status">DISCONNECTED{store.error.value && <span>{store.error.value}</span>}</div>}
      <UserHoverName hover={hover} />
      {!userlistHidden && !fullscreen && <a class={`${styles.copyright} ${left ? styles.leftCopyright : ''}`} href="/license" target="_blank" rel="noopener">Copyright (C) 2024 Vorlent</a>}
    </div>
  )
}
