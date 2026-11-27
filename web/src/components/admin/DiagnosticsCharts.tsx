import type { DiagAniList, DiagSample, DiagSummary } from '../../api'
import { SectionLabel } from '../ui/Section'
import { bytes, duration, plural } from './diagFormat'
import { TimeChart, type ChartPoint } from './TimeChart'
import styles from './DiagnosticsCharts.module.css'

// Where the watchdog calls the server stuck (diag.StuckLock and the rest).
const STUCK = { lockMs: 5000, dbMs: 3000, lagMs: 2000, requestMs: 15_000 }
export const ANILIST_LIMIT = 90 // calls a minute

// How close the server came to stuck: the worst of the four checks, as a
// share of where it counts as stuck.
export function closeness(s: DiagSample) {
  const parts = (Object.keys(STUCK) as (keyof typeof STUCK)[]).map((k) => s[k] === null ? null : s[k]! / STUCK[k] * 100)
  return parts.every((p) => p === null) ? null : Math.max(...parts.map((p) => p ?? 0))
}
const closenessNote = (s: DiagSample) => [
  s.lockMs !== null && `longest lock ${duration(s.lockMs)}`,
  s.dbMs !== null && `database ${duration(s.dbMs)}`,
  s.requestMs !== null && `slowest request ${duration(s.requestMs)}`,
  s.lagMs !== null && `watchdog ${duration(s.lagMs)} late`,
].filter(Boolean).join(' · ')

const mb = (v: number) => v >= 1000 ? `${Number((v / 1000).toFixed(1))} GB` : `${Math.round(v)} MB`

const health: {
  key: Exclude<keyof DiagSample, 'time'>; title: string; about: (s: DiagSummary) => string; format: (v: number) => string
  threshold?: (s: DiagSummary) => { value: number; label: string } | undefined
}[] = [
  { key: 'cpuPct', title: 'CPU', about: () => 'the server’s; 100 % is one core', format: (v) => `${Math.round(v)} %` },
  { key: 'memoryMb', title: 'Server memory', about: () => 'what the server itself holds', format: (v) => `${Math.round(v)} MB` },
  {
    key: 'hostFreeMb', title: 'Free memory on the machine', format: mb,
    about: (s) => {
      const total = Math.max(0, ...s.samples.map((x) => x.hostTotalMb ?? 0))
      return `the lowest; ${total ? `of ${Number((total / 1000).toFixed(1))} GB, ` : ''}the rooms’ browsers use most`
    },
  },
  {
    key: 'roomMemoryMb', title: 'Room memory', format: mb,
    about: () => 'the fullest room’s; at its cap a program there is ended',
    threshold: (s) => {
      const cap = Math.max(0, ...s.samples.map((x) => x.roomLimitMb ?? 0))
      return cap ? { value: cap, label: 'cap' } : undefined
    },
  },
  { key: 'people', title: 'People in rooms', about: () => 'the most at once', format: (v) => Math.round(v).toLocaleString() },
]

// Health, then AniList, over the time shown.
export function DiagnosticsCharts({ summary }: { summary: DiagSummary }) {
  const { since, until, bucketMs } = summary
  const per = duration(bucketMs)
  const samplesFrom = until - summary.sampleDays * 86_400_000
  const chart = { since, until, bucketMs }
  return (
    <div class={styles.charts}>
      <div>
        <SectionLabel>Health</SectionLabel>
        <p class={styles.about}>
          {summary.samples.length ? `The highest value per ${per}, from a sample every 10 seconds.` : 'No samples in this time.'}
          {summary.samples.length > 0 && since < samplesFrom && ` Samples are kept ${summary.sampleDays} days.`}
        </p>
        {summary.samples.length > 0 && <div class={styles.grid}>
          <TimeChart {...chart} title="How close to stuck" kind="line"
            description="the worst of: the longest-held lock, the database write check, the slowest request and how late the watchdog ran, each against where it counts as stuck"
            format={(v) => `${Math.round(v)} %`} threshold={{ value: 100, label: 'stuck' }}
            series={[{ name: 'How close to stuck', tone: 'line' }]}
            points={summary.samples.map((s) => ({ time: s.time, values: [closeness(s)], note: closenessNote(s) }))} />
          {health.map((h) => (
            <TimeChart key={h.key} {...chart} title={h.title} description={h.about(summary)} kind="line" format={h.format}
              threshold={h.threshold?.(summary)}
              series={[{ name: h.title, tone: 'line' }]}
              points={summary.samples.map((s) => ({ time: s.time, values: [s[h.key]] }))} />
          ))}
        </div>}
      </div>
      <div>
        <SectionLabel>AniList</SectionLabel>
        {summary.anilist.length === 0 ? <p class={styles.about}>No calls in this time.</p> : <>
          <p class={styles.about}>AniList answers about {ANILIST_LIMIT} calls a minute, then refuses (“too many”).</p>
          <div class={styles.grid}>
            <TimeChart {...chart} title="Busiest minute" description={`calls, per ${per}`} kind="columns"
              format={(v) => Math.round(v).toLocaleString()} threshold={{ value: ANILIST_LIMIT, label: 'AniList’s limit' }}
              series={[{ name: 'All answered', tone: 'base' }, { name: 'Some failed', tone: 'danger' }]}
              points={summary.anilist.map((b) => anilistPoint([b], b.time))} combine={(p, time) => anilistPoint(p.flatMap((x) => x.data as DiagAniList[]), time)} />
            <TimeChart {...chart} title="Data to and from AniList" description={`per ${per}`} kind="line" format={bytes}
              series={[{ name: 'Data', tone: 'line' }]}
              points={summary.anilist.map((b) => ({ time: b.time, values: [b.sentBytes + b.receivedBytes],
                note: `sent ${bytes(b.sentBytes)}, received ${bytes(b.receivedBytes)} in ${plural(b.calls, 'call', 'calls')}` }))} />
          </div>
        </>}
      </div>
    </div>
  )
}

// The busiest minute of these buckets, as a failed column if any call failed.
function anilistPoint(list: DiagAniList[], time: number): ChartPoint {
  const peak = Math.max(...list.map((b) => b.peak))
  const calls = list.reduce((a, b) => a + b.calls, 0), failed = list.reduce((a, b) => a + b.failed, 0)
  const limited = list.reduce((a, b) => a + b.limited, 0)
  return {
    time, data: list, values: failed ? [null, peak] : [peak, null],
    note: `${plural(calls, 'call', 'calls')} in all${failed ? `, ${failed} failed${limited ? ` (${limited} as too many)` : ''}` : ''}`,
  }
}
