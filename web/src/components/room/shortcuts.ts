export const shortcuts = [
  { action: 'fullscreen', label: 'Fullscreen on/off', keys: ['F'] },
  { action: 'mute', label: 'Mute/unmute', keys: ['M'] },
  { action: 'playback', label: 'Pause/play the stream', keys: ['P'] },
  { action: 'volumeUp', label: 'Volume up by 5', keys: ['ArrowUp'], repeat: true },
  { action: 'volumeDown', label: 'Volume down by 5', keys: ['ArrowDown'], repeat: true },
  { action: 'chat', label: 'Show/hide the chat', keys: ['C'] },
  { action: 'message', label: 'Write a message', keys: ['T'] },
  { action: 'users', label: 'Show/hide the user list', keys: ['U'] },
  { action: 'settings', label: 'Show this list', keys: ['?'] },
] as const

export type ShortcutAction = typeof shortcuts[number]['action']
export const shortcutKeys = (action: ShortcutAction) => shortcuts.find((shortcut) => shortcut.action === action)!.keys
export const shortcutLabel = (key: string) => key === 'ArrowUp' ? 'Arrow up' : key === 'ArrowDown' ? 'Arrow down' : key

interface ShortcutContext {
  enabled: boolean
  host: boolean
  editing: boolean
  dialog: boolean
  kicked: boolean
  inChat: boolean
  range: boolean
}

export function matchRoomShortcut(event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'repeat'>, context: ShortcutContext): ShortcutAction | null {
  if (!context.enabled || context.host || context.editing || context.dialog || context.kicked || event.ctrlKey || event.metaKey || event.altKey) return null
  const shortcut = shortcuts.find(({ keys }) => keys.some((key) => key.toLowerCase() === event.key.toLowerCase()))
  if (!shortcut || (event.repeat && !('repeat' in shortcut))) return null
  if ('repeat' in shortcut && (context.inChat || context.range)) return null
  return shortcut.action
}
