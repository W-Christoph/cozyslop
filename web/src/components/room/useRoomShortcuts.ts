import { useEffect } from 'preact/hooks'
import { preferences, settingsOpen, updatePreferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { matchRoomShortcut } from './shortcuts'

export function useRoomShortcuts({ windowOpen, onFullscreen, onChat, onMessage, onUsers }: {
  windowOpen: boolean
  onFullscreen: () => void
  onChat: () => void
  onMessage: () => void
  onUsers: () => void
}) {
  const store = useRoomStore()
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return // a control took the key, e.g. the More menu's arrows
      const target = event.target instanceof Element ? event.target : null
      const action = matchRoomShortcut(event, {
        enabled: preferences.value.shortcuts,
        host: store.isHost.value,
        editing: !!target?.closest('input, textarea, select') || !!target?.closest('[contenteditable]:not([contenteditable="false"])'),
        dialog: windowOpen || settingsOpen.value !== null || !!document.querySelector('[role="dialog"]'),
        kicked: !!store.kicked.value,
        inChat: !!target?.closest('aside[aria-label="Chat"]'),
        range: !!target?.matches('input[type="range"]'),
      })
      if (!action) return
      event.preventDefault()
      const { muted, volume } = preferences.value
      switch (action) {
        case 'fullscreen': onFullscreen(); break
        case 'mute': updatePreferences({ muted: !muted }); break
        case 'playback': store.paused.value ? store.resume() : store.pause(); break
        case 'volumeUp': updatePreferences({ volume: Math.min(100, volume + 5), muted: false }); break
        case 'volumeDown': updatePreferences({ volume: Math.max(0, volume - 5) }); break
        case 'chat': onChat(); break
        case 'message': onMessage(); break
        case 'users': onUsers(); break
        case 'settings': settingsOpen.value = 'shortcuts'; break
      }
    }
    window.addEventListener('keydown', keydown)
    return () => window.removeEventListener('keydown', keydown)
  }, [store, windowOpen, onFullscreen, onChat, onMessage, onUsers])
}
