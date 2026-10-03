import { useEffect, useMemo, useState } from 'preact/hooks'
import { api } from '../../../api'
import { Button } from '../../Button'
import { useRoomStore } from '../RoomContext'
import {
  bitrates,
  formatBitrate,
  formatScreen,
  parseScreen,
  parseStream,
  pickRate,
  pickStream,
  ratesFor,
  resolutions,
  scaledSize,
  scales,
  type Screen,
  type Stream,
} from './streamOptions'
import { useRoomSettingsForm } from './useRoomSettingsForm'
import styles from './StreamSettings.module.css'

const fields = ['screen', 'stream'] as const

interface Options {
  screens: Screen[]
  streams: Stream[] // the room's default first
}

export function StreamSettings() {
  const store = useRoomStore()
  const { draft, change, save, busy, error, message } = useRoomSettingsForm(fields)
  const [options, setOptions] = useState<Options | null>(null)
  const [loadError, setLoadError] = useState('')
  const [revision, setRevision] = useState(0)

  useEffect(() => {
    let active = true
    setLoadError('')
    void api
      .get<{ screens: string[]; streams: string[] }>(`/api/admin/rooms/${encodeURIComponent(store.room)}/stream-options`)
      .then((res) => {
        if (!active) return
        setOptions({
          screens: res.screens.map(parseScreen).filter((s): s is Screen => s !== null),
          streams: res.streams.map(parseStream).filter((s): s is Stream => s !== null),
        })
      })
      .catch((e) => active && setLoadError(e instanceof Error ? e.message : 'Something went wrong.'))
    return () => {
      active = false
    }
  }, [store.room, revision])

  const choices = useMemo(() => {
    if (!options || !draft) return null
    const screen = parseScreen(draft.screen)
    const resolution = screen ? `${screen.width}x${screen.height}` : ''
    // "" means neko's default stream, which is listed first.
    const stream = parseStream(draft.stream) ?? options.streams[0]
    return {
      screen,
      resolution,
      resolutions: resolutions(options.screens),
      rates: resolution ? ratesFor(options.screens, resolution) : [],
      stream,
      bitrates: bitrates(options.streams),
      scales: scales(options.streams),
    }
  }, [options, draft])

  const setResolution = (resolution: string) => {
    if (!options || !choices) return
    if (!resolution) return change('screen', '')
    const rate = pickRate(ratesFor(options.screens, resolution), choices.screen?.rate ?? 30)
    const [width, height] = resolution.split('x').map(Number)
    if (rate !== undefined) change('screen', formatScreen({ width, height, rate }))
  }
  const setRate = (rate: number) => {
    if (!choices?.screen) return
    change('screen', formatScreen({ ...choices.screen, rate }))
  }
  const setStream = (kbps: number, scale: number) => {
    if (!options) return
    const stream = pickStream(options.streams, kbps, scale)
    if (stream) change('stream', stream.id)
  }

  return (
    <form class={styles.form} onSubmit={(e) => { e.preventDefault(); void save() }}>
      <h2 class={styles.heading}>Stream Settings</h2>
      {loadError ? (
        <div>
          <p role="alert">{loadError}</p>
          <Button onClick={() => setRevision((v) => v + 1)}>Retry</Button>
        </div>
      ) : !choices ? (
        <p role="status">Loading…</p>
      ) : (
        <>
          <label class={styles.row}>Resolution
            <select value={choices.resolution} disabled={busy} onChange={(e) => setResolution(e.currentTarget.value)}>
              <option value="">Server default</option>
              {choices.resolutions.map((r) => <option key={r} value={r}>{r.replace('x', '×')}</option>)}
            </select>
          </label>
          {choices.screen && (
            <label class={styles.row}>Frame rate
              <select value={choices.screen.rate} disabled={busy} onChange={(e) => setRate(Number(e.currentTarget.value))}>
                {choices.rates.map((r) => <option key={r} value={r}>{r} fps</option>)}
              </select>
            </label>
          )}
          {choices.stream && (
            <>
              <label class={styles.row}>Bitrate
                <select value={choices.stream.kbps} disabled={busy}
                  onChange={(e) => setStream(Number(e.currentTarget.value), choices.stream!.scale)}>
                  {choices.bitrates.map((b) => <option key={b} value={b}>{formatBitrate(b)}</option>)}
                </select>
              </label>
              <label class={styles.row}>Stream size
                <select value={choices.stream.scale} disabled={busy}
                  onChange={(e) => setStream(choices.stream!.kbps, Number(e.currentTarget.value))}>
                  {choices.scales.map((s) => {
                    const size = choices.screen ? scaledSize(choices.screen.width, choices.screen.height, s) : null
                    return <option key={s} value={s}>{s === 100 ? 'Full' : `${s}%`}{size ? ` (${size[0]}×${size[1]})` : ''}</option>
                  })}
                </select>
              </label>
            </>
          )}
          <p class={styles.hint}>
            Higher bitrates look better and need more bandwidth per viewer. A smaller stream size is encoded
            at lower resolution: less CPU on the server and less bandwidth, at the cost of sharpness.
          </p>
        </>
      )}
      <Button type="submit" disabled={busy || !draft || !choices}>Update Stream Settings</Button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </form>
  )
}
