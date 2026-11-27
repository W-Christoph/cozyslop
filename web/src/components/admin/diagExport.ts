import { DIAG_GROUPS, type DiagEntry, type DiagFilter, type DiagSummary } from '../../api'
import { ANILIST_LIMIT, closeness } from './DiagnosticsCharts'
import { bytes, duration, entryLine, plural, stamp, zone } from './diagFormat'
import { incidents } from './Incidents'

// Admin > Diagnostics as text, for sending to whoever helps: what the page
// shows for the time chosen (the counts, what went wrong, every chart's
// values) and the log as filtered. Numbers as they are, not drawn.
export function diagnosticsText({ summary, stalls, log, filter, more }: {
  summary: DiagSummary
  stalls: DiagEntry[]
  log: DiagEntry[] // newest first
  filter: DiagFilter // the log's
  more: boolean // the log has older entries than these
}) {
  const s = summary, c = s.counts
  const out = [
    `CozyCast diagnostics, exported ${stamp(Date.now())} (times are ${zone()})`,
    `Time shown: ${stamp(s.since, false)} to ${stamp(s.until, false)}`,
    `Kept: ${bytes(s.sizeBytes)} on disk; entries ${s.retentionDays} days, health samples ${s.sampleDays} days; ` +
      `${s.dropped} entries dropped and ${s.tlsHandshakeErrors} failed TLS handshakes since the server started`,
  ]

  out.push('', '== Overview')
  const boot = s.lastBoot
  if (boot) {
    const crash = typeof boot.attrs?.crash === 'string' ? boot.attrs.crash : ''
    out.push(!boot.attrs?.previous ? `Running since ${stamp(boot.time)}.`
      : `Started ${stamp(boot.time)} after the server went down` +
        (boot.attrs.last_seen ? `; last seen ${stamp(Number(boot.attrs.last_seen))}` : '') +
        (crash ? `; Go ended it: ${crash}` : '; Go did not end it (out of memory, power, a kill)'))
  }
  out.push(`starts ${c.boots} (${c.crashes} after going down), stalls ${c.stalls}, errors ${c.errors}, warnings ${c.warnings}, ` +
    `slow or failed requests ${c.requests} (${c.failedRequests} failed)`,
  `AniList: ${c.anilist} calls, ${c.anilistFailed} failed (${c.anilistLimited} as too many), busiest minute ${c.anilistPeak} of ${ANILIST_LIMIT}, ` +
    `sent ${bytes(c.anilistSentBytes)}, received ${bytes(c.anilistReceivedBytes)}`)

  out.push('', '== What went wrong (newest first)')
  const list = incidents(stalls, s.boots)
  if (!list.length) out.push('Nothing: the server was never stuck or down.')
  for (const i of list) {
    out.push(i.kind === 'down' ? `DOWN ${stamp(i.time)}${i.until ? ` to ${stamp(i.until)} (${duration(i.until - i.time)})` : ''}`
      : `${i.kind === 'stuck' ? 'STUCK' : 'DUMP ON REQUEST'} ${stamp(i.time)}${i.lasted !== undefined ? ` for ${duration(i.lasted)}` : ''}`)
    if (i.note) out.push(`  ${i.note}`)
    for (const r of i.reasons) out.push(`  - ${r}`)
    if (i.dump) out.push(`  dump: ${i.dump}`)
  }

  const per = duration(s.bucketMs)
  out.push('', `== Health, the highest per ${per} (free memory the lowest), from a sample every 10 s`)
  if (!s.samples.length) out.push('No samples in this time.')
  else out.push(...table(['time', 'stuck%', 'cpu%', 'serverMB', 'freeMB', 'totalMB', 'roomMB', 'capMB', 'people', 'lockMs', 'dbMs', 'reqMs', 'lagMs'],
    s.samples.map((x) => [stamp(x.time, false), ...[closeness(x), x.cpuPct, x.memoryMb, x.hostFreeMb, x.hostTotalMb, x.roomMemoryMb,
      x.roomLimitMb, x.people, x.lockMs, x.dbMs, x.requestMs, x.lagMs].map(num)])))
  out.push('stuck% is the worst of lockMs (stuck at 5000), dbMs (3000), reqMs (15000) and lagMs (2000) against those.')

  out.push('', `== AniList per ${per}`)
  if (!s.anilist.length) out.push('No calls in this time.')
  else out.push(...table(['time', 'calls', 'failed', 'tooMany', 'busiestMinute', 'sentBytes', 'receivedBytes'],
    s.anilist.map((b) => [stamp(b.time, false), ...[b.calls, b.failed, b.limited, b.peak, b.sentBytes, b.receivedBytes].map(num)])))

  const groups = DIAG_GROUPS.filter((g) => filter.groups.includes(g.id)).map((g) => g.label).join(', ') || 'nothing'
  out.push('', `== Log: ${groups}; ${filter.level ? `${filter.level} and up` : 'any level'}` +
    `${filter.room ? `; room ${filter.room}` : ''}${filter.text.trim() ? `; text "${filter.text.trim()}"` : ''}` +
    `${filter.until ? `; ${stamp(filter.since)} to ${stamp(filter.until)}` : ''}; ${plural(log.length, 'entry', 'entries')}, newest first` +
    `${more ? ' (the newest only: there are older ones)' : ''}`)
  out.push(...log.map(entryLine), '')
  return out.join('\n')
}

const num = (v: number | null) => v === null ? '-' : String(Math.round(v))

// Rows under a heading, each column as wide as its widest value.
function table(head: string[], rows: string[][]) {
  const widths = head.map((h, i) => Math.max(h.length, ...rows.map((r) => r[i].length)))
  return [head, ...rows].map((r) => r.map((v, i) => i ? v.padStart(widths[i]) : v.padEnd(widths[i])).join('  '))
}
