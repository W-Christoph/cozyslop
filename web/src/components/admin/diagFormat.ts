import type { DiagEntry } from '../../api'
import type { Tone } from '../ui/Badge'

// How the diagnostics pages say times, sizes and levels.

const today = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
const earlier = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', second: '2-digit' })

// The time of day for today, with the date before.
export function when(ms: number) {
  return new Date(ms).toDateString() === new Date().toDateString() ? today.format(ms) : earlier.format(ms)
}

export function bytes(n: number) {
  if (n < 1024 * 1024) return `${Math.max(1, Math.round(n / 1024))} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

export function duration(ms: number) {
  if (ms < 1000) return `${ms} ms`
  if (ms < 60_000) return `${Number((ms / 1000).toFixed(ms < 10_000 ? 1 : 0))} s`
  const m = Math.floor(ms / 60_000), s = Math.round((ms % 60_000) / 1000)
  return s ? `${m} min ${s} s` : `${m} min`
}

// The level's name without what slog adds to it ("ERROR+2").
export function level(e: DiagEntry): { name: string; tone: Tone } {
  const name = e.level.replace(/[+-]\d+$/, '')
  return { name, tone: name === 'ERROR' ? 'danger' : name === 'WARN' ? 'warning' : 'neutral' }
}

export const KIND_NAMES: Record<string, string> = {
  event: 'Log', request: 'Request', outbound: 'Call', sample: 'Sample', stall: 'Stall', boot: 'Start', shutdown: 'Stop',
}

// An attribute's value as text: times as times, lists joined, numbers as
// they are.
export function attr(key: string, v: unknown): string {
  if (key === 'last_seen' && typeof v === 'number') return when(v)
  return value(v)
}

export function value(v: unknown): string {
  if (Array.isArray(v)) return v.map(value).join('; ')
  if (v !== null && typeof v === 'object') return JSON.stringify(v)
  return String(v)
}
