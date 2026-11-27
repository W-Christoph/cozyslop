import assert from 'node:assert/strict'
import test from 'node:test'
import { createServer } from 'vite'

process.env.TZ = 'UTC'
// Load the TypeScript module through Vite, like the other tests.
const server = await createServer({ configFile: false, server: { middlewareMode: true } })
let diagnosticsText
try {
  ;({ diagnosticsText } = await server.ssrLoadModule('/src/components/admin/diagExport.ts'))
} finally { await server.close() }

const at = (hhmm) => Date.UTC(2026, 10, 27, ...hhmm.split(':').map(Number))
const sample = (time, v) => ({ time: at(time), cpuPct: null, memoryMb: null, hostFreeMb: null, hostTotalMb: null, people: null,
  roomMemoryMb: null, roomLimitMb: null, lagMs: null, lockMs: null, dbMs: null, requestMs: null, ...v })
const boot = { id: 3, time: at('09:00'), kind: 'boot', level: 'INFO', msg: 'server started', attrs: { previous: 'no clean shutdown', last_seen: at('08:45') } }
const summary = {
  enabled: true, retentionDays: 7, sampleDays: 3, sizeBytes: 2_000_000, dropped: 0, tlsHandshakeErrors: 4,
  since: at('00:00'), until: at('12:00'), bucketMs: 600_000,
  counts: { errors: 1, warnings: 2, stalls: 1, requests: 0, failedRequests: 0, anilist: 12, anilistFailed: 1, anilistLimited: 1,
    anilistPeak: 9, anilistSentBytes: 1200, anilistReceivedBytes: 48_000, boots: 1, crashes: 1 },
  lastBoot: boot, boots: [boot],
  anilist: [{ time: at('10:00'), calls: 12, failed: 1, limited: 1, peak: 9, sentBytes: 1200, receivedBytes: 48_000 }],
  samples: [
    sample('10:00', { cpuPct: 4.4, memoryMb: 51, hostFreeMb: 2900, hostTotalMb: 7900, roomMemoryMb: 2100, roomLimitMb: 3120, people: 2, lockMs: 2500 }),
    sample('10:10', { memoryMb: 60, hostFreeMb: 412 }),
  ],
}
const stalls = [
  { id: 2, time: at('10:00:40'), kind: 'stall', level: 'WARN', msg: 'server well again', attrs: { duration_ms: 40_000, reasons: ['lock room default held 7.4s'] } },
  { id: 1, time: at('10:00'), kind: 'stall', level: 'ERROR', msg: 'server stuck', attrs: { reasons: ['lock room default held 5.1s'], dump: 'stall-1.txt.gz' } },
]
const log = [{ id: 9, time: at('11:00:05'), kind: 'event', level: 'WARN', room: 'default', msg: 'out of memory: programs ended', attrs: { ended: 1 } }]
const filter = { groups: ['log', 'stalls'], level: 'warn', text: '', room: '', since: summary.since }

test('the export has what the page shows, as numbers', () => {
  const text = diagnosticsText({ summary, stalls, log, filter, more: false })
  if (process.env.SHOW) console.log(text)
  for (const want of [
    'Time shown: 2026-11-27 00:00 to 2026-11-27 12:00',
    'Started 2026-11-27 09:00:00 after the server went down; last seen 2026-11-27 08:45:00; Go did not end it',
    'starts 1 (1 after going down), stalls 1, errors 1, warnings 2',
    'busiest minute 9 of 90',
    // What went wrong, newest first, in the page's words.
    'DOWN 2026-11-27 08:45:00 to 2026-11-27 09:00:00 (15 min)',
    'STUCK 2026-11-27 10:00:00 for 40 s\n',
    '  - Room “default” locked for 7.4 s\n  dump: stall-1.txt.gz',
    // Every bucket, columns as wide as their values; stuck% 2500 of 5000.
    'time              stuck%  cpu%  serverMB  freeMB  totalMB  roomMB  capMB  people  lockMs  dbMs  reqMs  lagMs\n' +
    '2026-11-27 10:00      50     4        51    2900     7900    2100   3120       2    2500     -      -      -\n' +
    '2026-11-27 10:10       -     -        60     412        -       -      -       -       -     -      -      -\n',
    '2026-11-27 10:00     12       1        1              9       1200          48000',
    '== Log: Log, Stalls; warn and up; 1 entry, newest first\n2026-11-27 11:00:05 WARN event room=default out of memory: programs ended {"ended":1}',
  ]) assert.ok(text.includes(want), `missing ${JSON.stringify(want)}\n${text}`)
  assert.match(diagnosticsText({ summary, stalls, log, filter, more: true }), /1 entry, newest first \(the newest only/)
})
