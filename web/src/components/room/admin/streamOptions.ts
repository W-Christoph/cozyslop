// Pure helpers for the stream settings form: what the room's desktop
// supports, split into the choices an admin makes one at a time.

export interface Screen {
  width: number
  height: number
  rate: number
}

export interface Stream {
  id: string // "b2500-s100"
  kbps: number
  scale: number // percent of the desktop size
}

export function parseScreen(value: string): Screen | null {
  const m = /^(\d+)x(\d+)@(\d+)$/.exec(value)
  return m ? { width: Number(m[1]), height: Number(m[2]), rate: Number(m[3]) } : null
}

export function formatScreen({ width, height, rate }: Screen): string {
  return `${width}x${height}@${rate}`
}

export function parseStream(id: string): Stream | null {
  const m = /^b(\d+)-s(\d+)$/.exec(id)
  return m ? { id, kbps: Number(m[1]), scale: Number(m[2]) } : null
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

/** The stream with this bitrate and size, or the closest one that exists. */
export function pickStream(streams: Stream[], kbps: number, scale: number): Stream | undefined {
  return (
    streams.find((s) => s.kbps === kbps && s.scale === scale) ??
    [...streams].sort(
      (a, b) =>
        Math.abs(a.scale - scale) - Math.abs(b.scale - scale) || Math.abs(a.kbps - kbps) - Math.abs(b.kbps - kbps),
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
