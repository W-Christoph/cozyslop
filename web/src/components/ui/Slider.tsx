import styles from './Slider.module.css'

// A range with labelled stops.
export function Slider({ label, value, stops, onChange, format = String }: {
  label: string
  value: number
  stops: number[] // ascending and evenly spaced
  onChange: (value: number) => void
  format?: (value: number) => string
}) {
  const min = stops[0], max = stops[stops.length - 1]
  const step = stops.length > 1 ? stops[1] - stops[0] : 1
  const position = (stop: number) => (stop - min) / (max - min)
  return (
    <div class={styles.slider}>
      <input type="range" aria-label={label} aria-valuetext={format(value)} min={min} max={max} step={step} value={value}
        style={{ '--fill': `${position(value) * 100}%` }}
        onInput={(e) => onChange(Number(e.currentTarget.value))} />
      <div class={styles.stops} aria-hidden="true">
        {stops.map((stop) => (
          <span key={stop} class={stop === value ? styles.current : undefined} style={{ '--at': position(stop) }}>{format(stop)}</span>
        ))}
      </div>
    </div>
  )
}
