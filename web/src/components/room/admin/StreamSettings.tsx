import { useEffect, useMemo, useState } from 'preact/hooks'
import { api } from '../../../api'
import { Button } from '../../Button'
import { Spinner } from '../../ui/Spinner'
import { Select } from '../../ui/Field'
import { FormActions } from '../../ui/FormActions'
import { Notice } from '../../ui/Notice'
import { Section, SettingRow } from '../../ui/Section'
import { useRoomStore } from '../RoomContext'
import {
  bitrates,
  formatBitrate,
  formatScreen,
  parseScreen,
  parseStream,
  pickRate,
  pickStream,
  presets,
  ratesFor,
  resolutions,
  scaledSize,
  scales,
  type Screen,
  type Stream,
} from './streamOptions'
import { useRoomSettingsForm } from './useRoomSettingsForm'

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
      presets: presets(options.streams),
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
  const setStream = (kbps: number, scale: number, preset: string) => {
    if (!options) return
    const stream = pickStream(options.streams, kbps, scale, preset)
    if (stream) change('stream', stream.id)
  }

  const select = (id: string) => `stream-${id}`
  return (
    <form onSubmit={(e) => { e.preventDefault(); void save() }}>
      <Section title="Video" description="What the room's desktop looks like and how it is sent to viewers. Applies to everyone at once.">
        {loadError ? (
          <>
            <Notice tone="error">{loadError}</Notice>
            <div><Button icon="refresh" onClick={() => setRevision((v) => v + 1)}>Retry</Button></div>
          </>
        ) : !choices ? (
          <Spinner label="Loading…" />
        ) : (
          <>
            <SettingRow title="Resolution" description="Of the desktop itself." htmlFor={select('resolution')}>
              <Select id={select('resolution')} value={choices.resolution} disabled={busy} onChange={(e) => setResolution(e.currentTarget.value)}>
                <option value="">Server default</option>
                {choices.resolutions.map((r) => <option key={r} value={r}>{r.replace('x', '×')}</option>)}
              </Select>
            </SettingRow>
            {choices.screen && (
              <SettingRow title="Frame rate" htmlFor={select('rate')}>
                <Select id={select('rate')} value={choices.screen.rate} disabled={busy} onChange={(e) => setRate(Number(e.currentTarget.value))}>
                  {choices.rates.map((r) => <option key={r} value={r}>{r} fps</option>)}
                </Select>
              </SettingRow>
            )}
            {choices.stream && (
              <>
                <SettingRow title="Bitrate" description="Higher looks better and needs more bandwidth per viewer." htmlFor={select('bitrate')}>
                  <Select id={select('bitrate')} value={choices.stream.kbps} disabled={busy}
                    onChange={(e) => setStream(Number(e.currentTarget.value), choices.stream!.scale, choices.stream!.preset)}>
                    {choices.bitrates.map((b) => <option key={b} value={b}>{formatBitrate(b)}</option>)}
                  </Select>
                </SettingRow>
                <SettingRow title="Stream size" description="A smaller stream is encoded at lower resolution: less CPU on the server and less bandwidth, at the cost of sharpness." htmlFor={select('scale')}>
                  <Select id={select('scale')} value={choices.stream.scale} disabled={busy}
                    onChange={(e) => setStream(choices.stream!.kbps, Number(e.currentTarget.value), choices.stream!.preset)}>
                    {choices.scales.map((s) => {
                      const size = choices.screen ? scaledSize(choices.screen.width, choices.screen.height, s) : null
                      return <option key={s} value={s}>{s === 100 ? 'Full' : `${s}%`}{size ? ` (${size[0]}×${size[1]})` : ''}</option>
                    })}
                  </Select>
                </SettingRow>
                {choices.presets.length > 1 && (
                  <SettingRow title="Encoder" description="A faster encoder saves CPU, but looks blockier at the same bitrate." htmlFor={select('preset')}>
                    <Select id={select('preset')} value={choices.stream.preset} disabled={busy}
                      onChange={(e) => setStream(choices.stream!.kbps, choices.stream!.scale, e.currentTarget.value)}>
                      {choices.presets.map((p, i, all) => (
                        <option key={p} value={p}>
                          {p[0].toUpperCase() + p.slice(1)}
                          {i === 0 ? ' (least CPU)' : i === all.length - 1 ? ' (sharpest)' : ''}
                        </option>
                      ))}
                    </Select>
                  </SettingRow>
                )}
              </>
            )}
          </>
        )}
      </Section>
      <FormActions error={error} message={message}>
        <Button variant="primary" type="submit" disabled={busy || !draft || !choices}>{busy ? 'Saving…' : 'Save changes'}</Button>
      </FormActions>
    </form>
  )
}
