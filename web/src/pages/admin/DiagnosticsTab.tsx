import { useEffect, useRef, useState } from 'preact/hooks'
import { adminDiagnostics, DIAG_GROUPS, DIAG_PAGE, type DiagEntry, type DiagFilter, type DiagnosticsSource, type DiagSummary } from '../../api'
import { Button } from '../../components/Button'
import { Modal } from '../../components/Modal'
import { DiagEntryList } from '../../components/admin/DiagEntryList'
import { DiagnosticsCharts } from '../../components/admin/DiagnosticsCharts'
import { DiagnosticsOverview, type Tile } from '../../components/admin/DiagnosticsOverview'
import { Incidents } from '../../components/admin/Incidents'
import { EmptyState } from '../../components/ui/EmptyState'
import { Select } from '../../components/ui/Field'
import { Notice } from '../../components/ui/Notice'
import { Section, SectionLabel } from '../../components/ui/Section'
import { Spinner } from '../../components/ui/Spinner'
import styles from './DiagnosticsTab.module.css'

const RANGES = [[6, 'Last 6 hours'], [24, 'Last 24 hours'], [72, 'Last 3 days'], [168, 'Last 7 days']] as const
// Room helpers poll every few seconds: shown when asked for.
const USUAL = DIAG_GROUPS.map((g) => g.id).filter((id) => id !== 'helpers')
const AROUND = 10 * 60_000
const message = (e: unknown) => e instanceof Error ? e.message : 'Something went wrong.'

type Shown = Omit<DiagFilter, 'since' | 'until'>

// Admin > Diagnostics. source is where the data comes from (made up in
// the snapshots).
export function DiagnosticsTab({ source = adminDiagnostics }: { source?: DiagnosticsSource }) {
  const [hours, setHours] = useState(24),
    [refresh, setRefresh] = useState(0),
    [summary, setSummary] = useState<DiagSummary | null>(null),
    [stalls, setStalls] = useState<DiagEntry[]>([]),
    [rooms, setRooms] = useState<string[]>([]),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(true)
  const [shown, setShown] = useState<Shown>({ groups: USUAL, level: '', text: '', room: '' }),
    [around, setAround] = useState<{ from: number; to: number } | null>(null),
    [text, setText] = useState(''), // the search, once typing paused
    [entries, setEntries] = useState<DiagEntry[]>([]),
    [entriesLoading, setEntriesLoading] = useState(true),
    [entriesError, setEntriesError] = useState(''),
    [more, setMore] = useState(false)
  const [clearing, setClearing] = useState(false),
    [busy, setBusy] = useState(false),
    [status, setStatus] = useState('')
  const incidentsRef = useRef<HTMLDivElement>(null), logRef = useRef<HTMLDivElement>(null)
  const range = (): Pick<DiagFilter, 'since' | 'until'> =>
    around ? { since: around.from, until: around.to } : { since: Date.now() - hours * 3_600_000 }

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void Promise.all([source.summary(hours), source.stalls(Date.now() - hours * 3_600_000)]).then(([s, st]) => {
      if (active) { setSummary(s); setStalls(st) }
    }).catch((e) => {
      if (active) setError(message(e))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [hours, refresh])

  useEffect(() => { void source.rooms().then(setRooms).catch(() => {}) }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => setText(shown.text), 300)
    return () => window.clearTimeout(timer)
  }, [shown.text])

  useEffect(() => {
    let active = true
    setEntriesLoading(true)
    setEntriesError('')
    void source.entries({ ...shown, text, ...range() }).then((list) => {
      if (active) { setEntries(list); setMore(list.length === DIAG_PAGE) }
    }).catch((e) => {
      if (active) setEntriesError(message(e))
    }).finally(() => {
      if (active) setEntriesLoading(false)
    })
    return () => { active = false }
  }, [hours, refresh, shown.groups, shown.level, shown.room, text, around])

  async function older() {
    setEntriesLoading(true)
    try {
      const list = await source.entries({ ...shown, text, ...range() }, entries.at(-1)?.id)
      setEntries((e) => [...e, ...list])
      setMore(list.length === DIAG_PAGE)
    } catch (e) {
      setEntriesError(message(e))
    } finally {
      setEntriesLoading(false)
    }
  }

  const toLog = () => requestAnimationFrame(() => logRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
  function pick(tile: Tile) {
    if (tile === 'stalls') {
      incidentsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
      return
    }
    setAround(null)
    setShown((s) => ({
      ...s,
      groups: tile === 'starts' ? ['starts'] : tile === 'requests' ? ['requests'] : tile === 'anilist' ? ['anilist'] : USUAL,
      level: tile === 'errors' ? 'error' : '',
    }))
    toLog()
  }
  function showAround(time: number) {
    setAround({ from: time - AROUND, to: time + AROUND })
    setShown((s) => ({ ...s, groups: DIAG_GROUPS.map((g) => g.id), level: '', room: '' }))
    toLog()
  }

  async function clear() {
    setBusy(true)
    try {
      await source.clear()
      setClearing(false)
      setStatus('Everything was deleted. The server keeps recording from now on.')
      setRefresh((r) => r + 1)
    } catch (e) {
      setError(message(e))
      setClearing(false)
    } finally {
      setBusy(false)
    }
  }

  const off = summary && !summary.enabled
  return (
    <Section title="Diagnostics"
      description="What the server kept to find out why it went down, got stuck or was slow. It names no one: no accounts, addresses, pages or shows."
      actions={!off && <>
        <Select compact aria-label="Time" value={String(hours)} onChange={(e) => { setAround(null); setHours(Number(e.currentTarget.value)) }}>
          {RANGES.map(([h, label]) => <option key={h} value={h}>{label}</option>)}
        </Select>
        <Button icon="refresh" disabled={loading} onClick={() => { setStatus(''); setRefresh((r) => r + 1) }}>Refresh</Button>
      </>}>
      {error && <Notice tone="error">{error}</Notice>}
      {status && <Notice tone="success">{status}</Notice>}
      {off ? <EmptyState icon="activity" title="Diagnostics are off">
        Set <code>DIAGNOSTICS=true</code> in <code>.env</code> and restart the server to keep them.
      </EmptyState> : <div class={styles.blocks}>
        {!summary && loading && <Spinner label="Loading diagnostics…" />}
        {summary && <div class={`${styles.blocks} ${loading ? styles.stale : ''}`}>
          <DiagnosticsOverview summary={summary} onPick={pick}
            actions={<Button size="sm" variant="danger-ghost" icon="trash" disabled={loading} onClick={() => setClearing(true)}>Delete all</Button>} />
          <div ref={incidentsRef} class={styles.anchor}>
            <SectionLabel>What went wrong</SectionLabel>
            <div class={styles.below}>
              <Incidents stalls={stalls} boots={summary.boots} loadDump={source.dump} onAround={showAround} />
            </div>
          </div>
          <DiagnosticsCharts summary={summary} />
        </div>}
        <div ref={logRef} class={styles.anchor}>
          <SectionLabel>Everything kept</SectionLabel>
          <div class={styles.below}>
            {entriesError && <Notice tone="error">{entriesError}</Notice>}
            <DiagEntryList entries={entries} rooms={rooms} loading={entriesLoading} more={more} onMore={older}
              filter={{ ...shown, ...range(), ...(around ? {} : { until: undefined }) }}
              onFilter={(change) => {
                if ('until' in change && change.until === undefined) setAround(null)
                const { since: _s, until: _u, ...rest } = change
                if (Object.keys(rest).length) setShown((s) => ({ ...s, ...rest }))
              }} />
          </div>
        </div>
      </div>}
      {clearing && <Modal title="Delete all diagnostics?" size="sm" onClose={() => { if (!busy) setClearing(false) }} footer={<>
        <Button disabled={busy} onClick={() => setClearing(false)}>Cancel</Button>
        <Button variant="danger" disabled={busy} onClick={clear}>{busy ? 'Deleting…' : 'Delete all'}</Button>
      </>}>
        <p class={styles.confirm}>Every entry and dump is deleted for good. The server keeps recording from now on.</p>
      </Modal>}
    </Section>
  )
}
