import { useEffect, useRef, useState } from 'preact/hooks'
import { Icon } from '../ui/Icon'
import styles from './ProgressCell.module.css'

// One person's progress in one show. Editable cells open a small editor:
// Enter or leaving it saves, Escape cancels.
export function ProgressCell({ episode, episodes, editable, label, onSave }: {
  episode: number // 0: not started
  episodes: number | null
  editable: boolean
  label: string // for screen readers: whose progress in what
  onSave: (episode: number) => void
}) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(episode)
  const wrapper = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  useEffect(() => { if (!editing) setValue(episode) }, [episode, editing])
  useEffect(() => { if (editing) input.current?.select() }, [editing])
  const max = episodes ?? 9999
  const clamp = (n: number) => Math.max(0, Math.min(max, Math.round(n) || 0))
  const finish = (save: boolean, to = value) => {
    setEditing(false)
    if (save && clamp(to) !== episode) onSave(clamp(to))
  }
  const shown = episode === 0 ? '—' : episodes ? `${episode} / ${episodes}` : String(episode)
  const done = episodes !== null && episode >= episodes
  if (!editing) {
    const text = <span class={`${styles.value} ${done ? styles.done : ''} ${episode === 0 ? styles.none : ''}`}>{shown}</span>
    return editable
      ? <button type="button" class={styles.cell} aria-label={`${label}: ${episode === 0 ? 'not started' : `episode ${episode}`}. Change`} onClick={() => setEditing(true)}>{text}</button>
      : <span class={styles.cell}>{text}</span>
  }
  return (
    <div ref={wrapper} class={styles.editor} role="group" aria-label={label}
      onFocusOut={(e) => { if (!wrapper.current?.contains(e.relatedTarget as Node | null)) finish(true) }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') { e.preventDefault(); finish(true) }
        else if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); finish(false) }
      }}>
      <button type="button" class={styles.step} aria-label="One less" disabled={value <= 0} onClick={() => setValue(clamp(value - 1))}>−</button>
      <input ref={input} class={styles.input} type="number" inputMode="numeric" min={0} max={max} value={value}
        aria-label="Last episode watched" onInput={(e) => setValue(clamp(e.currentTarget.valueAsNumber))} />
      <button type="button" class={styles.step} aria-label="One more" disabled={value >= max} onClick={() => setValue(clamp(value + 1))}>+</button>
      {episodes !== null && <button type="button" class={styles.text} onClick={() => finish(true, episodes)}>All</button>}
      <button type="button" class={styles.text} onClick={() => finish(true, 0)}>Clear</button>
      <button type="button" class={styles.save} aria-label="Save" onClick={() => finish(true)}><Icon name="check" size={16} /></button>
    </div>
  )
}
