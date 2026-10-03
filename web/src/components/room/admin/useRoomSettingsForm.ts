import { useEffect, useState } from 'preact/hooks'
import { api } from '../../../api'
import type { RoomSettings } from '../../../room/protocol'
import { useRoomStore } from '../RoomContext'

// Each form edits only its own fields. Full replacements must retain the
// latest values of every other setting, including screen and centerRemote.
export function useRoomSettingsForm(fields: readonly (keyof RoomSettings)[]) {
  const store = useRoomStore()
  const settings = store.settings.value
  const [draft, setDraft] = useState(settings)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  useEffect(() => { setDraft(settings) }, [settings])

  function change<K extends keyof RoomSettings>(field: K, value: RoomSettings[K]) {
    setDraft((current) => current && { ...current, [field]: value })
    setError('')
    setMessage('')
  }

  async function save() {
    const current = store.settings.value
    if (busy || !draft || !current || !store.rights.value.admin) return
    const edited = Object.fromEntries(fields.map((field) => [field, draft[field]]))
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const saved = await api.put<RoomSettings>(
        `/api/admin/rooms/${encodeURIComponent(store.room)}/settings`,
        { ...current, ...edited },
      )
      setDraft(saved)
      setMessage('Settings saved!')
      // The room_settings push updates the store for all viewers.
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return { draft, change, save, busy, error, message }
}
