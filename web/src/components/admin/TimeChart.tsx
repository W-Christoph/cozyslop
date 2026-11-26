import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import styles from './TimeChart.module.css'

// A value over time, per bucket of bucketMs from since to until: a line
// (one series), or stacked columns (several, bottom first). Hover or the
// arrow keys show a bucket's values; "Table" lists them all.

export interface ChartSeries {
  name: string
  tone: 'line' | 'base' | 'danger' // the accent's line, a neutral column, a failure
}
export interface ChartPoint {
  time: number // the bucket's start, unix ms
  values: (number | null)[] // one per series
  note?: string // shown under the values
  data?: unknown // what it was made from, for combine
}

const PAD = { top: 10, right: 10, bottom: 24, left: 52 }
const HEIGHT = 150

export function TimeChart({ title, description, since, until, bucketMs: bucket, kind, series, points: given, format, threshold, combine = sumPoints }: {
  title: string
  description?: string
  since: number
  until: number
  bucketMs: number
  kind: 'line' | 'columns'
  series: ChartSeries[]
  points: ChartPoint[]
  format: (value: number) => string
  threshold?: { value: number; label: string } // a line across, e.g. where "stuck" starts, once the values near it
  // Columns: several buckets as one, where they would be thinner than 3px.
  combine?: (points: ChartPoint[], time: number) => ChartPoint
}) {
  const frame = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(560)
  const [active, setActive] = useState<number | null>(null)
  useLayoutEffect(() => {
    const element = frame.current
    if (!element) return
    const measure = () => setWidth(Math.max(200, element.clientWidth))
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    measure()
    return () => observer.disconnect()
  }, [])

  const plotW = width - PAD.left - PAD.right, plotH = HEIGHT - PAD.top - PAD.bottom
  // Narrow: columns of several buckets each.
  const per = kind === 'columns' ? Math.max(1, Math.ceil(3 / (plotW * bucket / (until - since)))) : 1
  const bucketMs = bucket * per
  const points = per === 1 ? given : merged(given, since, bucketMs, combine)
  const total = (p: ChartPoint) => kind === 'columns' ? p.values.reduce<number>((a, v) => a + (v ?? 0), 0) : Math.max(...p.values.map((v) => v ?? 0))
  const highest = points.reduce((m, p) => Math.max(m, total(p)), 0)
  // The threshold only once the values come within half of it: far below,
  // its line would squash them to the bottom.
  const line = threshold && highest >= threshold.value / 2 ? threshold : undefined
  const top = Math.max(highest, line?.value ?? 0)
  const ticks = yTicks(top)
  const yMax = ticks[ticks.length - 1]
  const x = (t: number) => PAD.left + ((t - since) / (until - since)) * plotW
  const y = (v: number) => PAD.top + plotH - (v / yMax) * plotH
  const slot = Math.max(1, x(since + bucketMs) - x(since))

  const nearest = (px: number) => {
    let best = 0
    points.forEach((p, i) => {
      if (Math.abs(x(p.time + bucketMs / 2) - px) < Math.abs(x(points[best].time + bucketMs / 2) - px)) best = i
    })
    return best
  }
  const shown = active !== null && points[active] ? points[active] : null
  const peak = points.length ? points.reduce((a, p) => total(p) > total(a) ? p : a) : null
  const label = points.length
    ? `${title}: highest ${format(total(peak!))} at ${clock(peak!.time, until - since)}. Arrow keys go through the values; the table lists them.`
    : `${title}: nothing in this time.`

  return (
    <figure class={styles.chart}>
      <figcaption class={styles.head}>
        <span class={styles.title}>{title}</span>
        {description && <span class={styles.description}>{description}</span>}
        {series.length > 1 && <span class={styles.legend}>
          {series.map((s) => <span key={s.name} class={styles.key}><span class={`${styles.swatch} ${styles[s.tone]}`} />{s.name}</span>)}
        </span>}
      </figcaption>
      <div ref={frame} class={styles.plot} tabIndex={0} role="img" aria-label={label}
        onPointerMove={(e) => {
          if (!points.length) return
          const box = e.currentTarget.getBoundingClientRect()
          setActive(nearest(e.clientX - box.left))
        }}
        onPointerLeave={() => setActive(null)}
        onFocus={() => setActive((a) => a ?? (points.length ? points.length - 1 : null))}
        onBlur={() => setActive(null)}
        onKeyDown={(e) => {
          if (!points.length) return
          const step = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0
          if (e.key === 'Home') setActive(0)
          else if (e.key === 'End') setActive(points.length - 1)
          else if (step) setActive((a) => Math.min(points.length - 1, Math.max(0, (a ?? points.length - 1) + step)))
          else return
          e.preventDefault()
        }}>
        <svg width={width} height={HEIGHT} aria-hidden="true">
          {ticks.map((v) => <g key={v}>
            <line class={styles.grid} x1={PAD.left} x2={width - PAD.right} y1={y(v)} y2={y(v)} />
            <text class={styles.axis} x={PAD.left - 6} y={y(v)} dy="0.32em" text-anchor="end">{format(v)}</text>
          </g>)}
          {xTicks(since, until, plotW).map((t) => (
            <text key={t} class={styles.axis} x={x(t)} y={HEIGHT - 6} text-anchor="middle">{clock(t, until - since)}</text>
          ))}
          {line && <g>
            <line class={styles.threshold} x1={PAD.left} x2={width - PAD.right} y1={y(line.value)} y2={y(line.value)} />
            <text class={styles.thresholdLabel} x={PAD.left + 6} y={y(line.value) - 4}>{line.label}</text>
          </g>}
          {kind === 'line' ? lines(points, bucketMs, x, y) : columns(points, series, slot, x, y)}
          {shown && <line class={styles.crosshair} x1={x(shown.time + bucketMs / 2)} x2={x(shown.time + bucketMs / 2)} y1={PAD.top} y2={PAD.top + plotH} />}
          {shown && kind === 'line' && shown.values[0] !== null &&
            <circle class={styles.dot} cx={x(shown.time + bucketMs / 2)} cy={y(shown.values[0])} r={4} />}
        </svg>
        {!points.length && <div class={styles.empty}>Nothing in this time</div>}
        {shown && <div class={styles.tooltip} style={tooltipPlace(x(shown.time + bucketMs / 2), width)}>
          <div class={styles.when}>{span(shown.time, bucketMs, until - since)}</div>
          {series.map((s, i) => <div key={s.name} class={styles.row}>
            <span class={`${styles.lineKey} ${styles[s.tone]}`} />
            <strong>{shown.values[i] === null ? '–' : format(shown.values[i]!)}</strong>
            {series.length > 1 && <span class={styles.name}>{s.name}</span>}
          </div>)}
          {shown.note && <div class={styles.note}>{shown.note}</div>}
        </div>}
      </div>
      {points.length > 0 && <details class={styles.table}>
        <summary>Table</summary>
        <div class={styles.scroll}><table>
          <thead><tr><th scope="col">Time</th>{series.map((s) => <th key={s.name} scope="col">{s.name}</th>)}</tr></thead>
          <tbody>{points.map((p) => <tr key={p.time}>
            <td>{span(p.time, bucketMs, until - since)}</td>
            {p.values.map((v, i) => <td key={i}>{v === null ? '–' : format(v)}</td>)}
          </tr>)}</tbody>
        </table></div>
      </details>}
    </figure>
  )
}

// The line, broken where buckets are missing (the server was down), over
// a light wash.
function lines(points: ChartPoint[], bucketMs: number, x: (t: number) => number, y: (v: number) => number) {
  const runs: [number, number][][] = []
  let last = -Infinity
  for (const p of points) {
    const v = p.values[0]
    if (v === null) { last = -Infinity; continue }
    if (p.time - last > bucketMs * 1.5 || !runs.length) runs.push([])
    runs[runs.length - 1].push([x(p.time + bucketMs / 2), y(v)])
    last = p.time
  }
  const base = y(0)
  return runs.map((run, i) => run.length === 1
    ? <circle key={i} class={styles.dot} cx={run[0][0]} cy={run[0][1]} r={4} />
    : <g key={i}>
      <path class={styles.wash} d={`M${run[0][0]},${base}L${run.map(([a, b]) => `${a},${b}`).join('L')}L${run[run.length - 1][0]},${base}Z`} />
      <path class={styles.line} d={`M${run.map(([a, b]) => `${a},${b}`).join('L')}`} />
    </g>)
}

// Columns stacked bottom first, a 2px gap between neighbours and parts.
function columns(points: ChartPoint[], series: ChartSeries[], slot: number, x: (t: number) => number, y: (v: number) => number) {
  const w = Math.min(24, Math.max(1, slot - 2))
  return points.map((p) => {
    const left = x(p.time) + (slot - w) / 2
    let sum = 0
    const parts = p.values.map((v, i) => {
      if (!v) return null
      const bottom = y(sum) - (sum > 0 ? 2 : 0), top = y(sum + v)
      sum += v
      if (bottom - top < 0.5) return null
      const last = p.values.slice(i + 1).every((n) => !n)
      return <path key={i} class={styles[series[i].tone]} d={column(left, top, w, bottom - top, last && w >= 8 ? 4 : 0)} />
    })
    return <g key={p.time}>{parts}</g>
  })
}

// A column with its data end rounded (r) and its base square.
function column(left: number, top: number, w: number, h: number, r: number) {
  r = Math.min(r, h, w / 2)
  return `M${left},${top + h}V${top + r}Q${left},${top} ${left + r},${top}H${left + w - r}Q${left + w},${top} ${left + w},${top + r}V${top + h}Z`
}

// Round steps from 0 to at least max: 0, 1, 2, 5, 10, 20, 50...
function yTicks(max: number) {
  if (max <= 0) return [0, 1]
  const rough = max / 3
  const pow = 10 ** Math.floor(Math.log10(rough))
  // Whole steps from 1 up, so a count of 1 is not labelled 0, 1, 1.
  const step = Math.max(max >= 1 ? 1 : 0, [1, 2, 5, 10].map((m) => m * pow).find((s) => s >= rough)!)
  const ticks = [0]
  while (ticks[ticks.length - 1] < max) ticks.push(ticks[ticks.length - 1] + step)
  return ticks.map((t) => Number(t.toPrecision(12)))
}

// Whole local hours (or days) inside the range, at least 72px apart.
function xTicks(since: number, until: number, plotW: number) {
  const hours = (until - since) / 3_600_000
  const every = hours <= 6 ? 1 : hours <= 24 ? 4 : hours <= 72 ? 12 : 24
  const d = new Date(since)
  d.setMinutes(0, 0, 0)
  const list: number[] = []
  while (d.getTime() < until) {
    if (d.getTime() > since && d.getHours() % every === 0) list.push(d.getTime())
    d.setHours(d.getHours() + 1)
  }
  const room = Math.max(1, Math.floor(plotW / 72))
  const skip = Math.ceil(list.length / room)
  return list.filter((_, i) => i % skip === 0)
}

const time = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })
const day = new Intl.DateTimeFormat(undefined, { weekday: 'short' })
const dayTime = new Intl.DateTimeFormat(undefined, { weekday: 'short', hour: '2-digit', minute: '2-digit' })

// A time on an axis of this length: the hour, or the day at midnight and
// for long ones.
function clock(t: number, length: number) {
  if (new Date(t).getHours() === 0 && new Date(t).getMinutes() === 0) return day.format(t)
  return length <= 24 * 3_600_000 ? time.format(t) : dayTime.format(t)
}

// Buckets grouped into wider ones from since, each group made one point.
function merged(points: ChartPoint[], since: number, bucketMs: number, combine: (p: ChartPoint[], time: number) => ChartPoint) {
  const groups = new Map<number, ChartPoint[]>()
  for (const p of points) {
    const start = since + Math.floor((p.time - since) / bucketMs) * bucketMs
    groups.set(start, [...groups.get(start) ?? [], p])
  }
  return [...groups].map(([time, group]) => combine(group, time))
}

// Several buckets as one: the values added up.
function sumPoints(points: ChartPoint[], time: number): ChartPoint {
  return { time, values: points[0].values.map((_, i) => points.reduce<number | null>((a, p) => p.values[i] === null ? a : (a ?? 0) + p.values[i]!, null)) }
}

function span(t: number, bucketMs: number, length: number) {
  return `${length <= 24 * 3_600_000 ? time.format(t) : dayTime.format(t)}–${time.format(t + bucketMs)}`
}

// Beside the crosshair, on whichever side has room.
function tooltipPlace(px: number, width: number) {
  return px > width / 2 ? { right: `${width - px + 10}px` } : { left: `${px + 10}px` }
}
