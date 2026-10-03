// Pure helpers for the stream settings form: what the room's desktop
// supports, split into the choices an admin makes one at a time.

export interface Screen {
  width: number
  height: number
  rate: number
}

export interface Stream {
  id: string // "b2500-s100-veryfast"
  kbps: number
  scale: number // percent of the desktop size
  preset: string // x264 speed preset; "" if the id has none
}

// x264 speed presets, fastest (least CPU, softest picture) first.
const PRESET_ORDER = ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow', 'slower', 'veryslow']

export function parseScreen(value: string): Screen | null {
  const m = /^(\d+)x(\d+)@(\d+)$/.exec(value)
  return m ? { width: Number(m[1]), height: Number(m[2]), rate: Number(m[3]) } : null
}

export function formatScreen({ width, height, rate }: Screen): string {
  return `${width}x${height}@${rate}`
}

export function parseStream(id: string): Stream | null {
  const m = /^b(\d+)-s(\d+)(?:-([a-z]+))?$/.exec(id)
  return m ? { id, kbps: Number(m[1]), scale: Number(m[2]), preset: m[3] ?? '' } : null
}

/** Distinct resolutions as "WxH", largest first. */
export function resolutions(screens: Screen[]): string[] {
  const seen = new Map<string, Screen>()
  for (const s of screens) seen.set(`${s.width}x${s.height}`, s)
  return [...seen.entries()]
    .sort(([, a], [, b]) => b.width * b.height - a.width * a.height || b.width - a.width)
    .map(([key]) => key)
}

/** Frame rates available at a resolution ("WxH"), highest first. */
export function ratesFor(screens: Screen[], resolution: string): number[] {
  return [...new Set(screens.filter((s) => `${s.width}x${s.height}` === resolution).map((s) => s.rate))].sort(
    (a, b) => b - a,
  )
}

/**
 * The frame rate to keep after switching resolution: the current one if
 * still offered, else 30, else the closest lower one, else the lowest.
 */
export function pickRate(rates: number[], current: number): number | undefined {
  if (rates.includes(current)) return current
  if (rates.includes(30)) return 30
  return rates.find((r) => r < current) ?? rates[rates.length - 1]
}

export function bitrates(streams: Stream[]): number[] {
  return [...new Set(streams.map((s) => s.kbps))].sort((a, b) => a - b)
}

export function scales(streams: Stream[]): number[] {
  return [...new Set(streams.map((s) => s.scale))].sort((a, b) => b - a)
}

/** Presets on offer, fastest first; unknown names go last. */
export function presets(streams: Stream[]): string[] {
  const rank = (p: string) => (PRESET_ORDER.includes(p) ? PRESET_ORDER.indexOf(p) : PRESET_ORDER.length)
  return [...new Set(streams.map((s) => s.preset))].filter(Boolean).sort((a, b) => rank(a) - rank(b) || a.localeCompare(b))
}

/**
 * The stream with this bitrate, size and preset, or the closest one that
 * exists: same preset first, then the nearest size, then the nearest bitrate.
 */
export function pickStream(streams: Stream[], kbps: number, scale: number, preset: string): Stream | undefined {
  return (
    streams.find((s) => s.kbps === kbps && s.scale === scale && s.preset === preset) ??
    [...streams].sort(
      (a, b) =>
        Number(a.preset !== preset) - Number(b.preset !== preset) ||
        Math.abs(a.scale - scale) - Math.abs(b.scale - scale) ||
        Math.abs(a.kbps - kbps) - Math.abs(b.kbps - kbps),
    )[0]
  )
}

/** Encoded size at a scale, matching the worker's even-pixel rounding. */
export function scaledSize(width: number, height: number, scale: number): [number, number] {
  if (scale === 100) return [width, height]
  return [Math.round((width * scale) / 200) * 2, Math.round((height * scale) / 200) * 2]
}

export function formatBitrate(kbps: number): string {
  return kbps >= 1000 ? `${(kbps / 1000).toLocaleString()} Mbit/s` : `${kbps} kbit/s`
}
