import { useEffect, useState } from 'preact/hooks'
import { api } from '../../../api'
import { Button } from '../../Button'
import { useRoomStore } from '../RoomContext'
import { useRoomSettingsForm } from './useRoomSettingsForm'
import styles from './StreamSettings.module.css'

const fields = ['screen', 'quality'] as const

function dimensions(screen: string) {
  const match = /^(\d+)x(\d+)@(\d+)$/.exec(screen)
  return match ? match.slice(1).map(Number) : [0, 0, 0]
}

function screenLabel(screen: string) {
  const [width, height, rate] = dimensions(screen)
  return width ? `${width}×${height}, ${rate} fps` : screen
}

export function StreamSettings() {
  const store = useRoomStore()
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  const [screens, setScreens] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [screenError, setScreenError] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let active = true
    setLoading(true)
    setScreenError('')
    void api.get<string[]>(`/api/admin/rooms/${encodeURIComponent(store.room)}/screens`)
      .then((result) => {
        if (!active) return
        setScreens([...new Set(result.filter(Boolean))].sort((a, b) => {
          const [aw, ah, ar] = dimensions(a), [bw, bh, br] = dimensions(b)
          return aw * ah - bw * bh || aw - bw || ah - bh || ar - br
        }))
      })
      .catch((e) => {
        if (active) setScreenError(e instanceof Error ? e.message : 'Something went wrong.')
      })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [store.room, revision])

  return (
    <form class={styles.form} onSubmit={(e) => { e.preventDefault(); void save() }}>
      <h2 class={styles.heading}>Stream Settings</h2>
      {loading && <p role="status">Loading screens...</p>}
      {screenError ? (
        <div><p role="alert">{screenError}</p><Button onClick={() => setRevision((value) => value + 1)}>Retry screens</Button></div>
      ) : !loading && draft && (
        <label class={styles.row}>Screen
          <select value={draft.screen} disabled={busy} onChange={(e) => change('screen', e.currentTarget.value)}>
            <option value="">Server default</option>
            {draft.screen && !screens.includes(draft.screen) && <option value={draft.screen}>{screenLabel(draft.screen)} (current)</option>}
            {screens.map((screen) => <option key={screen} value={screen}>{screenLabel(screen)}</option>)}
          </select>
        </label>
      )}
      {draft && <label class={styles.row}>Quality
        <select value={draft.quality} disabled={busy} onChange={(e) => change('quality', e.currentTarget.value as typeof draft.quality)}>
          <option value="high">High</option><option value="medium">Medium</option><option value="low">Low</option>
        </select>
      </label>}
      <p class={styles.hint}>High uses the most bandwidth and CPU. Low is two-thirds resolution.</p>
      <Button type="submit" disabled={busy || !draft || loading || !!screenError}>Update Stream Settings</Button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </form>
  )
}
