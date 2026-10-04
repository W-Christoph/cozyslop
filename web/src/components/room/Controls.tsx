import { preferences, updatePreferences } from '../../app/state'
import { IconButton } from './IconButton'
import { useRoomStore } from './RoomContext'
import type { SidebarTab } from './Sidebar'
import styles from './Controls.module.css'

interface Props {
  fullscreen: boolean
  userlistHidden: boolean
  sidebar: SidebarTab
  onToggleUsers: () => void
  onPersonalSettings: () => void
  onSidebar: (tab: SidebarTab) => void
  onFullscreen: () => void
}

export function Controls({ fullscreen, userlistHidden, sidebar, onToggleUsers, onPersonalSettings, onSidebar, onFullscreen }: Props) {
  const store = useRoomStore()
  const host = store.isHost.value
  const holder = store.remoteHolder.value
  const holderName = holder ? store.users.value.get(holder)?.nickname ?? 'Someone' : ''
  const ownershipLocked = !!holder && !host && !!store.settings.value?.remoteOwnership
  const disabled = !store.rights.value.remote || ownershipLocked || store.video.value !== 'connected'
  const remoteLabel = host ? 'Drop remote' : holder ? 'Take remote' : 'Remote'
  const remoteTooltip = !store.rights.value.remote ? 'You are not allowed to use the remote'
    : ownershipLocked ? `${holderName} owns the remote` : remoteLabel
  const { muted, volume, userlistOnLeft } = preferences.value
  const toggleSidebar = (tab: SidebarTab) => onSidebar(sidebar === tab ? 'NOTHING' : tab)
  const usersIcon = fullscreen || userlistOnLeft
    ? userlistHidden ? 'chevron-right' : 'chevron-left'
    : userlistHidden ? 'chevron-up' : 'chevron-down'
  return (
    <div class={`${styles.controls} ${fullscreen ? styles.fullscreen : ''} ${fullscreen && host ? styles.host : ''}`}>
      {!(fullscreen && host) && <div class={styles.group}>
        <IconButton icon={usersIcon} label={userlistHidden ? 'Show Users' : 'Hide Users'} onClick={onToggleUsers} />
        <IconButton icon="settings" label="Personal settings" onClick={onPersonalSettings} />
        <a class={styles.home} href="/" aria-label="Home" title="Home"><img src="/svg/home.svg" alt="" /></a>
      </div>}
      <div class={styles.group}>
        {host && <IconButton icon="crosshair" label="Drop and center Remote" onClick={() => store.dropRemote(true)} />}
        <span class={styles.remote} title={remoteTooltip}>
          <IconButton icon="remoteAlpha" label={remoteLabel} title={remoteTooltip} active={host} disabled={!host && disabled}
            onClick={() => host ? store.dropRemote() : store.takeRemote()} />
        </span>
        <IconButton icon={store.paused.value ? 'play_button' : 'pause_button'} label={store.paused.value ? 'Play' : 'Pause'}
          active={store.paused.value} onClick={() => store.paused.value ? store.resume() : store.pause()} />
        <IconButton icon="fullscreen_button" label={fullscreen ? 'Exit fullscreen' : 'Fullscreen'} active={fullscreen} onClick={onFullscreen} />
        <IconButton icon={muted || volume === 0 ? 'sound-mute' : 'sound-max'} label={muted ? 'Unmute' : 'Mute'} active={muted}
          onClick={() => updatePreferences({ muted: !muted })} />
        <input aria-label="Volume" class={styles.volume} type="range" min="0" max="100" value={muted ? 0 : volume}
          onInput={(e) => {
            // Down to zero mutes and keeps the volume to come back to; moving
            // the slider up again unmutes.
            const value = Number(e.currentTarget.value)
            updatePreferences(value === 0 ? { muted: true } : { volume: value, muted: false })
          }} />
      </div>
      {!(fullscreen && host) && <div class={styles.group}>
        {store.rights.value.upload && <IconButton icon="imageupload" label="Files of the desktop" active={sidebar === 'FILES'}
          aria-pressed={sidebar === 'FILES'} onClick={() => toggleSidebar('FILES')} />}
        {store.rights.value.admin && <IconButton icon="settings" label="Room settings" active={sidebar === 'SETTINGS'}
          aria-pressed={sidebar === 'SETTINGS'} onClick={() => toggleSidebar('SETTINGS')} />}
        <IconButton icon="users" label="Users sidebar" active={sidebar === 'USERS'} aria-pressed={sidebar === 'USERS'} onClick={() => toggleSidebar('USERS')} />
        <IconButton icon="message-circle" label="Chat sidebar" active={sidebar === 'CHAT'} aria-pressed={sidebar === 'CHAT'} onClick={() => toggleSidebar('CHAT')} />
      </div>}
    </div>
  )
}
