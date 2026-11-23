import type { DiagEntry, DiagFilter, DiagSample, DiagSummary } from '../api'
import { DiagEntryList, KIND_GROUPS } from '../components/admin/DiagEntryList'
import { DiagnosticsCharts } from '../components/admin/DiagnosticsCharts'
import { DiagnosticsOverview } from '../components/admin/DiagnosticsOverview'
import { StallList } from '../components/admin/StallList'
import { EmptyState } from '../components/ui/EmptyState'
import { SectionLabel } from '../components/ui/Section'
import { Variants, type Scene } from './scene'
import styles from './Snapshots.module.css'

// Admin > Diagnostics: a day of a server that got stuck once and crashed
// once, a freshly cleared one, and one with diagnostics off.

const HOUR = 3_600_000
const now = Math.floor(Date.now() / HOUR) * HOUR
const since = now - 24 * HOUR
const bucketMs = 10 * 60_000
const stuckAt = now - 5 * HOUR - 20 * 60_000
const crashAt = now - 19 * HOUR

// The same made-up numbers every time.
let seed = 7
const random = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)

// Busier in the evening.
const busy = (t: number) => Math.max(0, Math.sin(((new Date(t).getHours() - 13) / 24) * 2 * Math.PI))

const samples: DiagSample[] = []
for (let t = since; t < now; t += bucketMs) {
  if (t > crashAt - 20 * 60_000 && t < crashAt) continue // down
  const near = Math.abs(t - stuckAt) < bucketMs
  const b = busy(t)
  samples.push({
    time: t,
    cpuPct: Math.round(6 + b * 30 + random() * 8 + (near ? 60 : 0)),
    memoryMb: Math.round((t < crashAt ? 70 + (t - since) / HOUR * 9 : 55 + (t - crashAt) / HOUR * 2) + random() * 4),
    goroutines: Math.round(120 + b * 260 + random() * 20),
    lagMs: near ? 2400 : Math.round(random() * 4),
    lockMs: near ? 7400 : Math.round(random() * 3 + b * 4),
    dbMs: near ? 5000 : Math.round(2 + random() * 6),
    schedMs: Math.round(random() * 2),
  })
}

const anilist: DiagSummary['anilist'] = []
for (let t = since; t < now; t += bucketMs) {
  const calls = Math.round(busy(t) * 14 * random())
  if (!calls) continue
  const failed = Math.abs(t - (now - 3 * HOUR)) < 2 * bucketMs ? Math.ceil(calls / 2) : 0
  anilist.push({ time: t, calls, failed, peak: Math.min(calls, 2 + Math.round(random() * 6)) })
}

const boot = (time: number, crashed: boolean): DiagEntry => ({
  id: 1, time, kind: 'boot', level: 'INFO', msg: 'server started',
  attrs: crashed ? { previous: 'no clean shutdown', last_seen: crashAt - 20 * 60_000 } : undefined,
})

const summary: DiagSummary = {
  enabled: true, retentionDays: 7, sampleDays: 3, sizeBytes: 14_600_000, dropped: 0, tlsHandshakeErrors: 1_843,
  since, until: now, bucketMs,
  counts: { errors: 6, warnings: 41, stalls: 1, requests: 12, failedRequests: 2, anilist: anilist.reduce((a, b) => a + b.calls, 0),
    anilistFailed: anilist.reduce((a, b) => a + b.failed, 0), boots: 1, crashes: 1 },
  lastBoot: boot(crashAt, true), anilist, samples,
}

const empty: DiagSummary = {
  ...summary, sizeBytes: 45_000, tlsHandshakeErrors: 0,
  counts: { errors: 0, warnings: 0, stalls: 0, requests: 0, failedRequests: 0, anilist: 0, anilistFailed: 0, boots: 0, crashes: 0 },
  lastBoot: boot(now - 3 * 86_400_000, false), anilist: [], samples: [],
}

const reasons = ['lock room default held 7.4s', 'db_main took 5.002s', 'request POST /api/rooms/{room}/media running 16s']
const stalls: DiagEntry[] = [
  { id: 30, time: stuckAt + 40_000, kind: 'stall', level: 'WARN', msg: 'server well again', attrs: { duration_ms: 40_000, reasons } },
  { id: 29, time: stuckAt, kind: 'stall', level: 'ERROR', msg: 'server stuck', attrs: { reasons: reasons.slice(0, 1), dump: `stall-${stuckAt}.txt.gz` } },
  { id: 12, time: now - 22 * HOUR, kind: 'stall', level: 'INFO', msg: 'dump', attrs: { reasons: ['SIGUSR1'], dump: `stall-${now - 22 * HOUR}.txt.gz` } },
]

// A goroutine as Go prints it in a dump.
const goroutine = (n: number, state: string) => `goroutine ${n} [${state}]:
cozycast/internal/hub.(*Room).post(0xc000a2e000, {0x1d3c0e8, 0xc0012f4a50}, {0xc000f0e1e0, 0x20}, {0x1a6b2c3, 0x4}, ...)
	/src/server/internal/hub/messages.go:236 +0x2a5
cozycast/internal/store.(*Store).InsertChat(0xc0001a8000, {0x1d3c0e8, 0xc0012f4a50}, 0xc000b7c1a0)
	/src/server/internal/store/chat.go:58 +0x1c8
database/sql.(*DB).ExecContext(0xc0001b2340, {0x1d3c0e8, 0xc0012f4a50}, {0x1a9f0a1, 0xd3}, {0xc000e1c0f0, 0xb, 0xb})
	/usr/local/go/src/database/sql/sql.go:1704 +0xc5
`
const dumpText = `CozyCast stall report, ${new Date(stuckAt).toISOString()}\nstuck: ${reasons[0]}\n\n` +
  `== Locks held\nroom default\t7.4s\n\n== Requests under way\nPOST /api/rooms/{room}/media\t16s\n\n` +
  `== Probes\ndb_main\tlast took 5.002s\ndb_watchlog\tlast took 3ms\n\n== goroutine profile\n` +
  Array.from({ length: 24 }, (_, i) => goroutine(100 + i * 3, i % 4 ? 'select' : 'sync.Mutex.Lock')).join('\n')

const longError = 'Post "https://graphql.anilist.co": read tcp 172.18.0.4:51234-><ip>:443: read: connection reset by peer; ' +
  'retrying after the server said: {"errors":[{"message":"Too Many Requests.","status":429,"locations":[{"line":1,"column":1}]}],"data":null} ' +
  'and the same again two more times before giving up, which is long enough to wrap several lines on a phone'
const entries: DiagEntry[] = [
  { id: 60, time: now - 4 * 60_000, kind: 'event', level: 'INFO', room: 'default', msg: 'neko ready' },
  { id: 59, time: now - 3 * HOUR, kind: 'outbound', level: 'WARN', msg: 'anilist search', attrs: { target: 'anilist', op: 'search', caller: 'watchlog', status: 429, retry_after: '60', rate_remaining: 0, duration_ms: 212 } },
  { id: 58, time: now - 3 * HOUR - 60_000, kind: 'event', level: 'ERROR', msg: 'anilist search failed', attrs: { err: longError } },
  { id: 57, time: now - 4 * HOUR, kind: 'outbound', level: 'INFO', room: 'default', msg: 'helper status (per minute)', attrs: { target: 'helper', op: 'status', count: 30, errors: 0, avg_ms: 4, max_ms: 19, status: 200 } },
  { id: 56, time: stuckAt + 40_000, kind: 'stall', level: 'WARN', msg: 'server well again', attrs: { duration_ms: 40_000, reasons } },
  { id: 55, time: stuckAt + 1_000, kind: 'request', level: 'INFO', room: 'default', msg: 'POST /api/rooms/{room}/media', attrs: { method: 'POST', route: 'POST /api/rooms/{room}/media', status: 204, duration_ms: 16_400 } },
  { id: 54, time: stuckAt, kind: 'stall', level: 'ERROR', msg: 'server stuck', attrs: { reasons: reasons.slice(0, 1), dump: `stall-${stuckAt}.txt.gz` } },
  { id: 53, time: now - 8 * HOUR, kind: 'event', level: 'WARN', room: 'home', msg: 'handshake did not complete after 5 seconds, retrying (try 2)', attrs: { component: 'wireguard' } },
  { id: 52, time: now - 12 * HOUR, kind: 'request', level: 'WARN', msg: 'GET /api/watchlog', attrs: { method: 'GET', route: 'GET /api/watchlog', status: 500, duration_ms: 31 } },
  { id: 51, time: crashAt, kind: 'boot', level: 'INFO', msg: 'server started', attrs: { previous: 'no clean shutdown', last_seen: crashAt - 20 * 60_000 } },
]

const filter: DiagFilter = { kinds: KIND_GROUPS.flatMap((g) => g.kinds), level: '', text: '', since }
const noDump = () => Promise.resolve('')

function Page({ data, list, stallList }: { data: DiagSummary; list: DiagEntry[]; stallList: DiagEntry[] }) {
  return (
    <div class={styles.stack}>
      <DiagnosticsOverview summary={data} />
      <div><SectionLabel>Stalls</SectionLabel><StallList stalls={stallList} loadDump={noDump} /></div>
      <DiagnosticsCharts summary={data} />
      <div><SectionLabel>Everything kept</SectionLabel>
        <DiagEntryList entries={list} filter={filter} loading={false} more={list.length > 0} onMore={() => {}} onFilter={() => {}} /></div>
    </div>
  )
}

export const diagnosticsScenes: Scene[] = [
  {
    name: 'diagnostics',
    about: 'A day with one stall and one crash: the counts, the stalls, AniList calls and health, and the log.',
    width: 1000,
    surface: 'page',
    render: () => <Page data={summary} list={entries} stallList={stalls} />,
  },
  {
    name: 'diagnostics-stall-dump',
    about: 'A stall with its dump shown: long lines scroll inside the box.',
    width: 1000,
    surface: 'page',
    render: () => <StallList stalls={stalls.slice(0, 2)} loadDump={noDump}
      open={{ [`stall-${stuckAt}.txt.gz`]: { state: 'shown', text: dumpText } }} />,
  },
  {
    name: 'diagnostics-empty',
    about: 'Just cleared: nothing counted and nothing to draw; and diagnostics switched off.',
    width: 1000,
    surface: 'page',
    render: () => (
      <Variants columns={1} items={[
        ['Cleared', <Page data={empty} list={[]} stallList={[]} />],
        ['Off', <EmptyState icon="activity" title="Diagnostics are off">
          Set <code>DIAGNOSTICS=true</code> in <code>.env</code> and restart the server to keep them.
        </EmptyState>],
      ]} />
    ),
  },
  {
    name: 'diagnostics-phone',
    about: 'The same day on a phone.',
    width: 390,
    surface: 'page',
    window: { height: 5200 },
    render: () => <Page data={summary} list={entries} stallList={stalls} />,
  },
]
