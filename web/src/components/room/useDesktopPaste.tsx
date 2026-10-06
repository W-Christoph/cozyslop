import { useCallback, useRef, useState } from 'preact/hooks'
import { preferences, updatePreferences } from '../../app/state'
import { useRoomStore } from './RoomContext'
import { PasteDialog } from './PasteDialog'

// Clipboard pastes need a preview; mobile typing also uses control/paste,
// but must not ask for confirmation for every character.
export function useDesktopPaste() {
  const store = useRoomStore()
  const [text, setText] = useState<string | null>(null)
  const pending = useRef(false)
  const requestPaste = useCallback((value: string) => {
    if (!store.isHost.value || !value || pending.current) return
    if (preferences.value.askBeforePaste === false) {
      store.neko.paste(value)
      return
    }
    pending.current = true
    setText(value)
  }, [store])
  const close = () => { pending.current = false; setText(null) }
  const dialog = text !== null && <PasteDialog text={text} onCancel={close} onAccept={(dontAsk) => {
    if (store.isHost.value) {
      if (dontAsk) updatePreferences({ askBeforePaste: false })
      store.neko.paste(text)
    }
    close()
  }} />
  return { requestPaste, pending, dialog }
}
