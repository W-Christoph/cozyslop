import type { DiagEntry, DiagFilter, DiagKind } from '../../api'
import { Button } from '../Button'
import { Badge } from '../ui/Badge'
import { Checkbox, Input, Select } from '../ui/Field'
import { attr, KIND_NAMES, level, when } from './diagFormat'
import styles from './DiagEntryList.module.css'

export const KIND_GROUPS: { label: string; kinds: DiagKind[] }[] = [
  { label: 'Log', kinds: ['event'] },
  { label: 'Requests', kinds: ['request'] },
  { label: 'AniList and helpers', kinds: ['outbound'] },
  { label: 'Stalls', kinds: ['stall'] },
  { label: 'Starts and stops', kinds: ['boot', 'shutdown'] },
]

// What the server kept, newest first, with filters above.
export function DiagEntryList({ entries, filter, onFilter, loading, more, onMore }: {
  entries: DiagEntry[]
  filter: DiagFilter
  onFilter: (change: Partial<DiagFilter>) => void
  loading: boolean
  more: boolean // a full page came: older entries may follow
  onMore: () => void
}) {
  const toggle = (kinds: DiagKind[], on: boolean) =>
    onFilter({ kinds: on ? [...filter.kinds, ...kinds] : filter.kinds.filter((k) => !kinds.includes(k)) })
  return (
    <div class={styles.entries}>
      <div class={styles.filters}>
        <fieldset class={styles.kinds}>
          <legend class={styles.hidden}>Kinds</legend>
          {KIND_GROUPS.map((g) => (
            <label key={g.label} class={styles.kind}>
              <Checkbox checked={g.kinds.every((k) => filter.kinds.includes(k))}
                onChange={(e) => toggle(g.kinds, e.currentTarget.checked)} />
              {g.label}
            </label>
          ))}
        </fieldset>
        <div class={styles.narrow}>
          <Select compact aria-label="Level" value={filter.level}
            onChange={(e) => onFilter({ level: e.currentTarget.value as DiagFilter['level'] })}>
            <option value="">Any level</option>
            <option value="warn">Warnings and errors</option>
            <option value="error">Errors only</option>
          </Select>
          <Input compact type="search" placeholder="Search text" aria-label="Search text" value={filter.text}
            onInput={(e) => onFilter({ text: e.currentTarget.value })} />
        </div>
      </div>
      {!loading && !entries.length && <p class={styles.none}>
        {filter.kinds.length ? 'Nothing matches in this time.' : 'Choose what to show.'}
      </p>}
      {entries.length > 0 && <ol class={`${styles.list} ${loading ? styles.stale : ''}`} aria-busy={loading}>
        {entries.map((e) => {
          const l = level(e)
          const attrs = Object.entries(e.attrs ?? {})
          return (
            <li key={e.id} class={styles.entry}>
              <time class={styles.time}>{when(e.time)}</time>
              <span class={styles.tags}>
                <Badge tone={l.tone}>{l.name}</Badge>
                <span class={styles.kindName}>{KIND_NAMES[e.kind] ?? e.kind}</span>
                {e.room && <span class={styles.room}>{e.room}</span>}
              </span>
              <div class={styles.body}>
                <div class={styles.msg}>{e.msg}</div>
                {attrs.length > 0 && <dl class={styles.attrs}>
                  {attrs.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{attr(k, v)}</dd></div>)}
                </dl>}
              </div>
            </li>
          )
        })}
      </ol>}
      {more && <Button class={styles.more} disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Show older'}</Button>}
    </div>
  )
}
