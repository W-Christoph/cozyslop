import { useEffect, useState } from 'preact/hooks'
import { adminDiagnostics, DIAG_PAGE, type DiagEntry, type DiagFilter, type DiagSummary } from '../../api'
import { Button } from '../../components/Button'
import { Modal } from '../../components/Modal'
import { DiagEntryList, KIND_GROUPS } from '../../components/admin/DiagEntryList'
import { DiagnosticsCharts } from '../../components/admin/DiagnosticsCharts'
import { DiagnosticsOverview } from '../../components/admin/DiagnosticsOverview'
import { StallList } from '../../components/admin/StallList'
import { EmptyState } from '../../components/ui/EmptyState'
import { Select } from '../../components/ui/Field'
import { Notice } from '../../components/ui/Notice'
import { Section, SectionLabel } from '../../components/ui/Section'
import { Spinner } from '../../components/ui/Spinner'
import styles from './DiagnosticsTab.module.css'

const RANGES = [[6, 'Last 6 hours'], [24, 'Last 24 hours'], [72, 'Last 3 days'], [168, 'Last 7 days']] as const
const ALL_KINDS = KIND_GROUPS.flatMap((g) => g.kinds)
const message = (e: unknown) => e instanceof Error ? e.message : 'Something went wrong.'

export function DiagnosticsTab() {
  const [hours, setHours] = useState(24),
    [refresh, setRefresh] = useState(0),
    [summary, setSummary] = useState<DiagSummary | null>(null),
    [stalls, setStalls] = useState<DiagEntry[]>([]),
    [error, setError] = useState(''),
    [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState<Omit<DiagFilter, 'since'>>({ kinds: ALL_KINDS, level: '', text: '' }),
    [text, setText] = useState(''), // the search, once typing paused
    [entries, setEntries] = useState<DiagEntry[]>([]),
    [entriesLoading, setEntriesLoading] = useState(true),
    [entriesError, setEntriesError] = useState(''),
    [more, setMore] = useState(false)
  const [clearing, setClearing] = useState(false),
    [busy, setBusy] = useState(false),
    [status, setStatus] = useState('')
  const since = () => Date.now() - hours * 3_600_000

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void Promise.all([adminDiagnostics.summary(hours), adminDiagnostics.stalls(since())]).then(([s, st]) => {
      if (active) { setSummary(s); setStalls(st) }
    }).catch((e) => {
      if (active) setError(message(e))
    }).finally(() => {
      if (active) setLoading(false)
    })
    return () => { active = false }
  }, [hours, refresh])

  useEffect(() => {
    const timer = window.setTimeout(() => setText(filter.text), 300)
    return () => window.clearTimeout(timer)
  }, [filter.text])

  useEffect(() => {
    let active = true
    setEntriesLoading(true)
    setEntriesError('')
    const load = filter.kinds.length
      ? adminDiagnostics.entries({ ...filter, text, since: since() })
      : Promise.resolve([])
    void load.then((list) => {
      if (active) { setEntries(list); setMore(list.length === DIAG_PAGE) }
    }).catch((e) => {
      if (active) setEntriesError(message(e))
    }).finally(() => {
      if (active) setEntriesLoading(false)
    })
    return () => { active = false }
  }, [hours, refresh, filter.kinds, filter.level, text])

  async function older() {
    setEntriesLoading(true)
    try {
      const list = await adminDiagnostics.entries({ ...filter, text, since: since() }, entries[entries.length - 1]?.id)
      setEntries((e) => [...e, ...list])
      setMore(list.length === DIAG_PAGE)
    } catch (e) {
      setEntriesError(message(e))
    } finally {
      setEntriesLoading(false)
    }
  }

  async function clear() {
    setBusy(true)
    try {
      await adminDiagnostics.clear()
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
      description="What the server kept to find out why it was slow or stuck. It names no one: no accounts, addresses or pages."
      actions={<>
        <Button icon="refresh" disabled={loading} onClick={() => { setStatus(''); setRefresh((r) => r + 1) }}>Refresh</Button>
        {!off && <Button variant="danger-ghost" icon="trash" disabled={loading} onClick={() => setClearing(true)}>Delete all</Button>}
      </>}>
      {error && <Notice tone="error">{error}</Notice>}
      {status && <Notice tone="success">{status}</Notice>}
      {off ? <EmptyState icon="activity" title="Diagnostics are off">
        Set <code>DIAGNOSTICS=true</code> in <code>.env</code> and restart the server to keep them.
      </EmptyState> : <div class={styles.blocks}>
        <div class={styles.range}>
          <Select compact aria-label="Time" value={String(hours)} onChange={(e) => setHours(Number(e.currentTarget.value))}>
            {RANGES.map(([h, label]) => <option key={h} value={h}>{label}</option>)}
          </Select>
          {loading && summary && <Spinner inline label="Loading…" />}
        </div>
        {!summary && loading && <Spinner label="Loading diagnostics…" />}
        {summary && <div class={`${styles.blocks} ${loading ? styles.stale : ''}`}>
          <DiagnosticsOverview summary={summary} />
          <div>
            <SectionLabel>Stalls</SectionLabel>
            <div class={styles.below}><StallList stalls={stalls} loadDump={adminDiagnostics.dump} /></div>
          </div>
          <DiagnosticsCharts summary={summary} />
        </div>}
        <div>
          <SectionLabel>Everything kept</SectionLabel>
          <div class={styles.below}>
            {entriesError && <Notice tone="error">{entriesError}</Notice>}
            <DiagEntryList entries={entries} filter={{ ...filter, since: 0 }} loading={entriesLoading} more={more} onMore={older}
              onFilter={(change) => setFilter((f) => ({ ...f, ...change }))} />
          </div>
        </div>
      </div>}
      {clearing && <Modal title="Delete all diagnostics?" size="sm" onClose={() => { if (!busy) setClearing(false) }} footer={<>
        <Button disabled={busy} onClick={() => setClearing(false)}>Cancel</Button>
        <Button variant="danger" disabled={busy} onClick={clear}>{busy ? 'Deleting…' : 'Delete all'}</Button>
      </>}>
        <p class={styles.confirm}>Every entry and stall dump is deleted for good. The server keeps recording from now on.</p>
      </Modal>}
    </Section>
  )
}
