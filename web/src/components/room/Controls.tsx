import { preferences, updatePreferences } from '../../app/state'
import { shortcutKeys } from './shortcuts'
import { RoomTooltip } from './RoomTooltip'
import { MoreMenu } from './MoreMenu'
import { IconButton } from './IconButton'
import { useRoomStore } from './RoomContext'
import type { RoomWindow } from '../../pages/RoomPage'
import styles from './Controls.module.css'

interface Props {
  fullscreen: boolean
  userlistHidden: boolean
  chatOpen: boolean
  unreadChat: number
  window: RoomWindow | null
  onToggleUsers: () => void
  onPersonalSettings: () => void
  onChat: (open: boolean) => void
  onWindow: (window: RoomWindow) => void
  onFullscreen: () => void
}

export function Controls({ fullscreen, userlistHidden, chatOpen, unreadChat, window, onToggleUsers, onPersonalSettings, onChat, onWindow, onFullscreen }: Props) {
  const store = useRoomStore()
  const host = store.isHost.value
  const holder = store.remoteHolder.value
  const holderName = holder ? store.users.value.get(holder)?.nickname ?? 'Someone' : ''
  const ownershipLocked = !!holder && !host && !!store.settings.value?.remoteOwnership
  const disabled = !store.rights.value.remote || ownershipLocked || store.video.value !== 'connected'
  const remoteLabel = host ? 'Drop remote' : holder ? 'Take remote' : 'Remote'
  const remoteTooltip = !store.rights.value.remote ? 'You are not allowed to use the remote'
    : ownershipLocked ? `${holderName} owns the remote` : remoteLabel
  const { muted, volume } = preferences.value
  const upload = store.desktopUpload.value
  const uploading = upload.state === 'uploading'
  return (
    <div class={`${styles.controls} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && host ? styles.host : ''}`}>
      {!(fullscreen && host) && <div class={`${styles.group} ${styles.navigation}`}>
        <RoomTooltip label="Home"><a class={styles.home} href="/" aria-label="Home"><img src="/svg/home.svg" alt="" /></a></RoomTooltip>
        {store.rights.value.upload && <IconButton class={`${styles.quiet} ${window === 'files' ? styles.on : uploading ? styles.uploading : ''}`} icon="folder" label="Files of the desktop"
          tooltip={uploading ? upload.message : undefined} style={uploading ? { '--fill': `${Math.round(upload.progress * 100)}%` } : undefined}
          aria-haspopup="dialog" onClick={() => onWindow('files')} />}
        {store.rights.value.admin && <IconButton class={`${styles.quiet} ${window === 'settings' ? styles.on : ''}`} icon="room-settings" label="Room settings"
          aria-haspopup="dialog" onClick={() => onWindow('settings')} />}
      </div>}
      <div class={`${styles.group} ${styles.center}`}>
        {host && <IconButton class={`${styles.desktopAction} ${styles.dropCenter}`} icon="crosshair" label="Drop and center Remote" onClick={() => store.dropRemote(true)} />}
        <RoomTooltip label={remoteTooltip}><span class={styles.remote}>
          <IconButton icon="remoteAlpha" label={remoteLabel} tooltip={false} active={host} disabled={!host && disabled}
            onClick={() => host ? store.dropRemote() : store.takeRemote()} />
        </span></RoomTooltip>
        <IconButton shortcut="playback" icon={store.paused.value ? 'play_button' : 'pause_button'} label={store.paused.value ? 'Play' : 'Pause'}
          active={store.paused.value} onClick={() => store.paused.value ? store.resume() : store.pause()} />
        <IconButton shortcut="mute" icon={muted || volume === 0 ? 'sound-mute' : 'sound-max'} label={muted ? 'Unmute' : 'Mute'} active={muted}
          onClick={() => updatePreferences({ muted: !muted })} />
        <input aria-label="Volume" aria-keyshortcuts={preferences.value.shortcuts ? [...shortcutKeys('volumeUp'), ...shortcutKeys('volumeDown')].join(' ') : undefined} class={styles.volume} type="range" min="0" max="100" value={muted ? 0 : volume} style={{ '--fill': `${muted ? 0 : volume}%` }}
          onInput={(e) => {
            // Down to zero mutes and keeps the volume to come back to; moving
            // the slider up again unmutes.
            const value = Number(e.currentTarget.value)
            updatePreferences(value === 0 ? { muted: true } : { volume: value, muted: false })
          }} />
        <IconButton shortcut="fullscreen" icon="fullscreen_button" label={fullscreen ? 'Exit fullscreen' : 'Fullscreen'} active={fullscreen} onClick={onFullscreen} />
      </div>
      {!(fullscreen && host) && <div class={`${styles.group} ${styles.right}`}>
        <IconButton class={`${styles.quiet} ${styles.sideOnly}`} icon="settings" label="Personal settings" onClick={onPersonalSettings} />
        <IconButton class={`${styles.quiet} ${styles.sideOnly} ${userlistHidden ? '' : styles.on}`} icon="users" shortcut="users"
          label={userlistHidden ? 'Show Users' : 'Hide Users'} aria-pressed={!userlistHidden} onClick={onToggleUsers} />
        <IconButton shortcut="chat" icon="message-circle" label={chatOpen ? 'Hide chat' : `Show chat${unreadChat ? `, ${unreadChat} new messages` : ''}`}
          class={`${styles.quiet} ${chatOpen ? styles.on : ''}`} aria-pressed={chatOpen} badge={chatOpen ? 0 : unreadChat} onClick={() => onChat(!chatOpen)} />
        {!fullscreen && <MoreMenu userlistHidden={userlistHidden} onToggleUsers={onToggleUsers} onPersonalSettings={onPersonalSettings} onWindow={onWindow} />}
      </div>}
    </div>
  )
}
