import { DIAG_GROUPS, type DiagAniList, type DiagEntry, type DiagnosticsSource, type DiagSample, type DiagSummary } from '../api'
import { Incidents } from '../components/admin/Incidents'
import { DiagnosticsTab } from '../pages/admin/DiagnosticsTab'
import type { Scene } from './scene'

// Admin > Diagnostics: a day on which the server got stuck once, crashed
// once and was killed once when the machine ran out of memory; a freshly
// cleared one; and one with diagnostics off.

const HOUR = 3_600_000
const now = Math.floor(Date.now() / HOUR) * HOUR
const since = now - 24 * HOUR
const bucketMs = 10 * 60_000
const stuckAt = now - 5 * HOUR - 20 * 60_000
const panicAt = now - 19 * HOUR // Go ended it
const killedAt = now - 9 * HOUR // the machine ran out of memory
const down = 15 * 60_000

// The same made-up numbers every time.
let seed = 7
const random = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)

// Busier in the evening.
const busy = (t: number) => Math.max(0, Math.sin(((new Date(t).getHours() - 13) / 24) * 2 * Math.PI))
const isDown = (t: number) => (t > panicAt - down && t < panicAt) || (t > killedAt - down && t < killedAt)

const samples: DiagSample[] = []
for (let t = since; t < now; t += bucketMs) {
  if (isDown(t)) continue
  const near = Math.abs(t - stuckAt) < bucketMs
  const b = busy(t)
  const beforeKill = t < killedAt && t > killedAt - 2 * HOUR ? (t - (killedAt - 2 * HOUR)) / (2 * HOUR) : 0
  samples.push({
    time: t,
    cpuPct: Math.round(6 + b * 30 + random() * 8 + (near ? 60 : 0)),
    memoryMb: Math.round(60 + b * 30 + random() * 6),
    hostFreeMb: Math.max(40, Math.round(2600 - b * 1400 - beforeKill * 2500 + random() * 120)),
    hostTotalMb: 7900,
    people: Math.round(b * 9 + random()),
    roomMemoryMb: Math.min(6900, Math.round(1200 + b * 2600 + beforeKill * 3000 + random() * 100)),
    roomLimitMb: 6900,
    lagMs: near ? 2400 : Math.round(random() * 4),
    lockMs: near ? 7400 : Math.round(random() * 3 + b * 4),
    dbMs: near ? 5000 : Math.round(2 + random() * 6),
    requestMs: near ? 16_400 : Math.round(random() * 40),
  })
}

const anilist: DiagAniList[] = []
for (let t = since; t < now; t += bucketMs) {
  const calls = Math.round(busy(t) * 14 * random())
  if (!calls || isDown(t)) continue
  const failed = Math.abs(t - (now - 3 * HOUR)) < 2 * bucketMs ? Math.ceil(calls / 2) : 0
  anilist.push({ time: t, calls, failed, limited: failed, peak: Math.min(calls, 2 + Math.round(random() * 6)),
    sentBytes: calls * 380, receivedBytes: calls * (2500 + Math.round(random() * 4000)) })
}
// One busy evening minute near the limit.
anilist[8].peak = 74

const panicDump = `crash-${panicAt}.txt.gz`
const boots: DiagEntry[] = [
  { id: 80, time: killedAt, kind: 'boot', level: 'INFO', msg: 'server started', attrs: { previous: 'no clean shutdown', last_seen: killedAt - down } },
  { id: 40, time: panicAt, kind: 'boot', level: 'INFO', msg: 'server started',
    attrs: { previous: 'no clean shutdown', last_seen: panicAt - down, crash: 'fatal error: concurrent map writes', dump: panicDump } },
]

const sum = (key: keyof DiagAniList) => anilist.reduce((a, b) => a + b[key], 0)
const summary: DiagSummary = {
  enabled: true, retentionDays: 7, sampleDays: 3, sizeBytes: 14_600_000, dropped: 0, tlsHandshakeErrors: 1_843,
  since, until: now, bucketMs,
  counts: { errors: 6, warnings: 41, stalls: 1, requests: 12, failedRequests: 2, anilist: sum('calls'), anilistFailed: sum('failed'),
    anilistLimited: sum('limited'), anilistPeak: 74, anilistSentBytes: sum('sentBytes'), anilistReceivedBytes: sum('receivedBytes'), boots: 2, crashes: 2 },
  lastBoot: boots[0], boots, anilist, samples,
}

const reasons = ['lock room default held 7.4s', 'db_main took 5.002s', 'request POST /api/rooms/{room}/media running 16s']
const stalls: DiagEntry[] = [
  { id: 70, time: stuckAt + 40_000, kind: 'stall', level: 'WARN', msg: 'server well again', attrs: { duration_ms: 40_000, reasons } },
  { id: 69, time: stuckAt, kind: 'stall', level: 'ERROR', msg: 'server stuck', attrs: { reasons: reasons.slice(0, 1), dump: `stall-${stuckAt}.txt.gz` } },
]

// A goroutine as Go prints it in a dump.
const goroutine = (n: number, state: string) => `goroutine ${n} [${state}]:
cozycast/internal/hub.(*Room).post(0xc000a2e000, {0x1d3c0e8, 0xc0012f4a50}, {0xc000f0e1e0, 0x20}, {0x1a6b2c3, 0x4}, ...)
\t/src/server/internal/hub/messages.go:236 +0x2a5
cozycast/internal/store.(*Store).InsertChat(0xc0001a8000, {0x1d3c0e8, 0xc0012f4a50}, 0xc000b7c1a0)
\t/src/server/internal/store/chat.go:58 +0x1c8
database/sql.(*DB).ExecContext(0xc0001b2340, {0x1d3c0e8, 0xc0012f4a50}, {0x1a9f0a1, 0xd3}, {0xc000e1c0f0, 0xb, 0xb})
\t/usr/local/go/src/database/sql/sql.go:1704 +0xc5
`
const dumpText = `CozyCast stall report, ${new Date(stuckAt).toISOString()}\nstuck: ${reasons[0]}\n\n` +
  `== Locks held\nroom default\t7.4s\n\n== Requests under way\nPOST /api/rooms/{room}/media\t16s\n\n` +
  `== Probes\ndb_main\tlast took 5.002s\ndb_watchlog\tlast took 3ms\n\n== goroutine profile\n` +
  Array.from({ length: 24 }, (_, i) => goroutine(100 + i * 3, i % 4 ? 'select' : 'sync.Mutex.Lock')).join('\n')

const longError = 'Post "https://graphql.anilist.co": read tcp 172.18.0.4:51234-><ip>:443: read: connection reset by peer; ' +
  'retrying after the server said: {"errors":[{"message":"Too Many Requests.","status":429,"locations":[{"line":1,"column":1}]}],"data":null} ' +
  'and the same again two more times before giving up, which is long enough to wrap several lines on a phone'
const helperPoll = (id: number, time: number): DiagEntry => ({ id, time, kind: 'outbound', level: 'INFO', room: 'default', msg: 'helper status (per minute)',
  attrs: { target: 'helper', op: 'status', count: 30, errors: 0, avg_ms: 4, max_ms: 19, status: 200 } })
const entries = ([
  { id: 99, time: now - 4 * 60_000, kind: 'event', level: 'INFO', room: 'default', msg: 'neko ready' },
  helperPoll(98, now - 5 * 60_000), helperPoll(97, now - 6 * 60_000),
  { id: 96, time: now - 3 * HOUR, kind: 'outbound', level: 'WARN', msg: 'anilist search', attrs: { target: 'anilist', op: 'search', caller: 'room_detect', status: 429, retry_after: '60', rate_remaining: 0, duration_ms: 212, sent_bytes: 412, received_bytes: 96 } },
  { id: 95, time: now - 3 * HOUR - 60_000, kind: 'event', level: 'ERROR', msg: 'anilist search failed', attrs: { err: longError } },
  { id: 94, time: now - 3 * HOUR - 2 * 60_000, kind: 'event', level: 'INFO', room: 'default', msg: 'anime detection', attrs: { outcome: 'search' } },
  { id: 93, time: now - 3 * HOUR - 3 * 60_000, kind: 'outbound', level: 'INFO', msg: 'anilist search', attrs: { target: 'anilist', op: 'search', caller: 'room_detect', status: 200, rate_remaining: 61, duration_ms: 188, sent_bytes: 412, received_bytes: 5821 } },
  ...stalls,
  { id: 68, time: stuckAt + 1_000, kind: 'request', level: 'INFO', room: 'default', msg: 'POST /api/rooms/{room}/media', attrs: { method: 'POST', route: 'POST /api/rooms/{room}/media', status: 204, duration_ms: 16_400 } },
  boots[0],
  { id: 79, time: killedAt - down - 30_000, kind: 'event', level: 'WARN', room: 'home', msg: 'handshake did not complete after 5 seconds, retrying (try 2)', attrs: { component: 'wireguard' } },
  { id: 52, time: now - 12 * HOUR, kind: 'request', level: 'WARN', msg: 'GET /api/watchlog', attrs: { method: 'GET', route: 'GET /api/watchlog', status: 500, duration_ms: 31 } },
  boots[1],
] as DiagEntry[]).sort((a, b) => b.time - a.time)

// The data above, filtered roughly as the server does.
function fake(data: DiagSummary, list: DiagEntry[], incidents: DiagEntry[], rooms = ['default', 'home']): DiagnosticsSource {
  const levels: Record<string, number> = { DEBUG: -4, INFO: 0, WARN: 4, ERROR: 8 }
  return {
    summary: async () => data,
    stalls: async () => incidents,
    entries: async (f) => {
      const groups = DIAG_GROUPS.filter((g) => f.groups.includes(g.id))
      return list.filter((e) => groups.some((g) => g.kinds.includes(e.kind) && (!g.target || e.attrs?.target === g.target)) &&
        (!f.level || levels[e.level] >= (f.level === 'warn' ? 4 : 8)) && (!f.room || e.room === f.room) &&
        e.time >= f.since && (!f.until || e.time < f.until))
    },
    dump: async () => dumpText,
    rooms: async () => rooms,
    clear: async () => {},
  }
}

const empty: DiagSummary = {
  ...summary, sizeBytes: 45_000, tlsHandshakeErrors: 0,
  counts: Object.fromEntries(Object.keys(summary.counts).map((k) => [k, 0])) as DiagSummary['counts'],
  lastBoot: { ...boots[0], time: now - 3 * 86_400_000, attrs: {} }, boots: [], anilist: [], samples: [],
}

export const diagnosticsScenes: Scene[] = [
  {
    name: 'diagnostics',
    about: 'A day with a crash, a kill when the machine ran out of memory, and a stall: the whole page.',
    width: 1100,
    surface: 'page',
    render: () => <DiagnosticsTab source={fake(summary, entries, stalls)} />,
  },
  {
    name: 'diagnostics-stall-dump',
    about: 'A stall with its dump shown: long lines scroll inside the box.',
    width: 1000,
    surface: 'page',
    render: () => <Incidents stalls={stalls} boots={[]} loadDump={async () => dumpText} onAround={() => {}}
      open={{ [`stall-${stuckAt}.txt.gz`]: { state: 'shown', text: dumpText } }} />,
  },
  {
    name: 'diagnostics-empty',
    about: 'Just cleared: nothing counted and nothing to draw.',
    width: 1000,
    surface: 'page',
    render: () => <DiagnosticsTab source={fake(empty, [], [], ['default'])} />,
  },
  {
    name: 'diagnostics-off',
    about: 'Diagnostics switched off.',
    width: 1000,
    surface: 'page',
    render: () => <DiagnosticsTab source={fake({ ...empty, enabled: false }, [], [])} />,
  },
  {
    name: 'diagnostics-phone',
    about: 'The same day on a phone.',
    width: 390,
    surface: 'page',
    window: { height: 5600 },
    render: () => <DiagnosticsTab source={fake(summary, entries, stalls)} />,
  },
]
