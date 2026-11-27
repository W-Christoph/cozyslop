import type { DiagEntry } from '../../api'
import type { Tone } from '../ui/Badge'

// How the diagnostics pages say times, sizes, levels and reasons.

const clock = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
const dated = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' })
const dayName = new Intl.DateTimeFormat(undefined, { weekday: 'long', day: 'numeric', month: 'long' })

const sameDay = (a: number, b: number) => new Date(a).toDateString() === new Date(b).toDateString()

// The time of day for today, with the date before.
export function when(ms: number) {
  return sameDay(ms, Date.now()) ? clock.format(ms) : dated.format(ms)
}

export const timeOfDay = (ms: number) => clock.format(ms)

// From one time to another, the date said once.
export const span = (from: number, to: number) => `${when(from)} to ${sameDay(from, to) ? clock.format(to) : when(to)}`

// "Today", "Yesterday" or the date: a heading over a day's entries.
export function day(ms: number) {
  if (sameDay(ms, Date.now())) return 'Today'
  if (sameDay(ms, Date.now() - 86_400_000)) return 'Yesterday'
  return dayName.format(ms)
}

export function bytes(n: number) {
  if (n < 1000) return `${Math.round(n)} B`
  if (n < 1_000_000) return `${Math.round(n / 1000)} kB`
  return `${Number((n / 1_000_000).toFixed(1))} MB`
}

export function duration(ms: number) {
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(ms < 10_000 ? 1 : 0))} s`
  const m = Math.floor(ms / 60_000), s = Math.round((ms % 60_000) / 1000)
  return s ? `${m} min ${s} s` : `${m} min`
}

export const plural = (n: number, one: string, many: string) => `${n.toLocaleString()} ${n === 1 ? one : many}`

// The level's name without what slog adds to it ("ERROR+2").
export function level(e: DiagEntry): { name: string; tone: Tone } {
  const name = e.level.replace(/[+-]\d+$/, '')
  return { name, tone: name === 'ERROR' ? 'danger' : name === 'WARN' ? 'warning' : 'neutral' }
}

export const KIND_NAMES: Record<string, string> = {
  event: 'Log', request: 'Request', outbound: 'Call', sample: 'Sample', stall: 'Stall', boot: 'Start', shutdown: 'Stop',
}

// An entry's attributes worth showing, as text: what its message already
// says is left out; durations, sizes and times are written out.
export function attrs(e: DiagEntry): [string, string][] {
  const repeated = new Set<string>()
  if (e.kind === 'request') repeated.add('route').add('method')
  for (const k of ['target', 'op']) if (typeof e.attrs?.[k] === 'string' && e.msg.includes(e.attrs[k] as string)) repeated.add(k)
  return Object.entries(e.attrs ?? {}).filter(([k]) => !repeated.has(k)).map(([k, v]) => [k, attr(k, v)])
}

export function attr(key: string, v: unknown): string {
  if (typeof v === 'number') {
    if (key === 'last_seen') return when(v)
    if (key.endsWith('_ms')) return duration(v)
    if (key.endsWith('_bytes')) return bytes(v)
  }
  if (key === 'reasons' && Array.isArray(v)) return v.map((r) => reason(String(r))).join('; ')
  return value(v)
}

export function value(v: unknown): string {
  if (Array.isArray(v)) return v.map(value).join('; ')
  if (v !== null && typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

// Go's duration ("7.4s", "5.002s", "350ms", "1m2s") in milliseconds.
function goDuration(text: string) {
  let ms = 0
  for (const [, n, unit] of text.matchAll(/([\d.]+)(h|ms|µs|m|s)/g)) {
    ms += Number(n) * ({ h: 3_600_000, m: 60_000, s: 1000, ms: 1, µs: 0.001 } as Record<string, number>)[unit]
  }
  return ms
}

const probes: Record<string, string> = { db_main: 'Database write check', db_watchlog: 'Watchlog database write check' }

// Why the watchdog thought the server stuck (diag/watchdog.go), in the
// words the charts use.
export function reason(r: string): string {
  let m: RegExpMatchArray | null
  if ((m = r.match(/^lock (.+) held (\S+)$/))) {
    const name = m[1] === 'hub' ? 'The list of rooms'
      : m[1].endsWith(' chat') ? `The chat of room “${m[1].slice(5, -5)}”` : `Room “${m[1].slice(5)}”`
    return `${name} locked for ${duration(goDuration(m[2]))}`
  }
  if ((m = r.match(/^(db_\w+) (took|pending) (\S+)$/))) {
    return `${probes[m[1]] ?? m[1]} ${m[2] === 'took' ? 'took' : 'waiting for'} ${duration(goDuration(m[3]))}`
  }
  if ((m = r.match(/^(db_\w+) failed: (.*)$/))) return `${probes[m[1]] ?? m[1]} failed: ${m[2]}`
  if ((m = r.match(/^request (.+) running (\S+)$/))) return `Request ${m[1]} running for ${duration(goDuration(m[2]))}`
  if ((m = r.match(/^watchdog (\S+) late$/))) return `The watchdog ran ${duration(goDuration(m[1]))} late`
  return r
}

// Local time as "2026-11-27 13:10:05" (seconds) or "2026-11-27 13:10",
// for exports; zone() says which.
export function stamp(ms: number, seconds = true) {
  const local = new Date(ms - new Date(ms).getTimezoneOffset() * 60_000).toISOString().replace('T', ' ')
  return local.slice(0, seconds ? 19 : 16)
}

// The browser's offset from UTC now, as "+01:00".
export function zone() {
  const offset = -new Date().getTimezoneOffset(), abs = Math.abs(offset)
  return `${offset < 0 ? '-' : '+'}${String(Math.floor(abs / 60)).padStart(2, '0')}:${String(abs % 60).padStart(2, '0')}`
}

// An entry as one line of text, for exports: its attributes as the server
// keeps them.
export function entryLine(e: DiagEntry) {
  return [stamp(e.time), e.level, e.kind, e.room ? `room=${e.room}` : '', e.msg, e.attrs ? JSON.stringify(e.attrs) : '']
    .filter(Boolean).join(' ')
}

// Has the browser save text as a file.
export function saveText(text: string, name: string) {
  const a = document.createElement('a')
  a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  a.download = name
  a.click()
  URL.revokeObjectURL(a.href)
}

// "2026-11-27-1803", for file names.
export const fileTime = () => new Date().toISOString().slice(0, 16).replace('T', '-').replace(':', '')
