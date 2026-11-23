import type { DiagSample, DiagSummary } from '../../api'
import { SectionLabel } from '../ui/Section'
import { duration } from './diagFormat'
import { TimeChart } from './TimeChart'
import styles from './DiagnosticsCharts.module.css'

const health: { key: Exclude<keyof DiagSample, 'time'>; title: string; about: string; format: (v: number) => string; stuck?: number }[] = [
  { key: 'cpuPct', title: 'CPU', about: '100 % is one core', format: (v) => `${Math.round(v)} %` },
  { key: 'memoryMb', title: 'Memory', about: 'what Go holds', format: (v) => `${Math.round(v)} MB` },
  { key: 'goroutines', title: 'Goroutines', about: 'grows with people and rooms', format: (v) => Math.round(v).toLocaleString() },
  { key: 'lockMs', title: 'Longest-held lock', about: 'how long one was held', format: (v) => duration(Math.round(v)), stuck: 5000 },
  { key: 'dbMs', title: 'Database write check', about: 'taking cozycast.db’s write lock', format: (v) => duration(Math.round(v)), stuck: 3000 },
  { key: 'lagMs', title: 'Watchdog late', about: 'beyond its 5 s round', format: (v) => duration(Math.round(v)), stuck: 2000 },
]

// AniList calls, and the health samples, over the time shown.
export function DiagnosticsCharts({ summary }: { summary: DiagSummary }) {
  const { since, until, bucketMs } = summary
  const per = duration(bucketMs)
  const samplesFrom = until - summary.sampleDays * 86_400_000
  return (
    <div class={styles.charts}>
      {summary.anilist.length === 0 ? <div>
        <SectionLabel>AniList calls</SectionLabel>
        <p class={styles.about}>None in this time.</p>
      </div> : <TimeChart title="AniList calls" description={`per ${per}; AniList allows about 90 a minute`} kind="columns"
        since={since} until={until} bucketMs={bucketMs} format={(v) => v.toLocaleString()}
        series={[{ name: 'Answered', tone: 'base' }, { name: 'Failed', tone: 'danger' }]}
        points={summary.anilist.map((b) => ({ time: b.time, values: [b.calls - b.failed, b.failed], note: `Busiest minute: ${b.peak}` }))} />}
      <div>
        <SectionLabel>Health</SectionLabel>
        <p class={styles.about}>
          {summary.samples.length ? `The highest value per ${per}, from a sample every 10 seconds.` : 'No samples in this time.'}
          {summary.samples.length > 0 && since < samplesFrom && ` Samples are kept ${summary.sampleDays} days.`}
        </p>
        {summary.samples.length > 0 && <div class={styles.grid}>
          {health.map((h) => (
            <TimeChart key={h.key} title={h.title} description={h.about} kind="line"
              since={since} until={until} bucketMs={bucketMs} format={h.format}
              threshold={h.stuck ? { value: h.stuck, label: `stuck from ${duration(h.stuck)}` } : undefined}
              series={[{ name: h.title, tone: 'line' }]}
              points={summary.samples.map((s) => ({ time: s.time, values: [s[h.key]] }))} />
          ))}
        </div>}
      </div>
    </div>
  )
}
