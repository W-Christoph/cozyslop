import type { DiagEntry, DiagFilter } from '../../api'
import { DIAG_GROUPS } from '../../api'
import { Button } from '../Button'
import { Badge } from '../ui/Badge'
import { Checkbox, Input, Select } from '../ui/Field'
import { attrs, day, KIND_NAMES, level, timeOfDay, when } from './diagFormat'
import styles from './DiagEntryList.module.css'

// What the server kept, newest first under a heading per day, with
// filters above.
export function DiagEntryList({ entries, filter, rooms, onFilter, loading, more, onMore }: {
  entries: DiagEntry[]
  filter: DiagFilter
  rooms: string[]
  onFilter: (change: Partial<DiagFilter>) => void
  loading: boolean
  more: boolean // a full page came: older entries may follow
  onMore: () => void
}) {
  const toggle = (id: string, on: boolean) =>
    onFilter({ groups: on ? [...filter.groups, id] : filter.groups.filter((g) => g !== id) })
  const days: [string, DiagEntry[]][] = []
  for (const e of entries) {
    const d = day(e.time)
    if (days.at(-1)?.[0] !== d) days.push([d, []])
    days.at(-1)![1].push(e)
  }
  return (
    <div class={styles.entries}>
      <div class={styles.filters}>
        <fieldset class={styles.kinds}>
          <legend class={styles.hidden}>What to show</legend>
          {DIAG_GROUPS.map((g) => (
            <label key={g.id} class={styles.kind}>
              <Checkbox checked={filter.groups.includes(g.id)} onChange={(e) => toggle(g.id, e.currentTarget.checked)} />
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
          {rooms.length > 1 && <Select compact aria-label="Room" value={filter.room} onChange={(e) => onFilter({ room: e.currentTarget.value })}>
            <option value="">All rooms</option>
            {rooms.map((r) => <option key={r} value={r}>{r}</option>)}
          </Select>}
          <Input compact type="search" placeholder="Search text" aria-label="Search text" value={filter.text}
            onInput={(e) => onFilter({ text: e.currentTarget.value })} />
        </div>
      </div>
      {filter.until && <div class={styles.around}>
        <span>Only from {when(filter.since)} to {timeOfDay(filter.until)}</span>
        <Button size="sm" variant="ghost" icon="x" aria-label="Show the whole time again" onClick={() => onFilter({ since: 0, until: undefined })} />
      </div>}
      {!loading && !entries.length && <p class={styles.none}>
        {filter.groups.length ? 'Nothing matches in this time.' : 'Choose what to show.'}
      </p>}
      {entries.length > 0 && <div class={`${styles.days} ${loading ? styles.stale : ''}`} aria-busy={loading}>
        {days.map(([d, list]) => (
          <section key={d}>
            <h4 class={styles.day}>{d}</h4>
            <ol class={styles.list}>
              {list.map((e) => {
                const l = level(e)
                const shown = attrs(e)
                return (
                  <li key={e.id} class={styles.entry}>
                    <time class={styles.time}>{timeOfDay(e.time)}</time>
                    <span class={styles.tags}>
                      <Badge tone={l.tone}>{l.name}</Badge>
                      <span class={styles.kindName}>{KIND_NAMES[e.kind] ?? e.kind}</span>
                      {e.room && <span class={styles.room}>{e.room}</span>}
                    </span>
                    <div class={styles.body}>
                      <div class={styles.msg}>{e.msg}</div>
                      {shown.length > 0 && <dl class={styles.attrs}>
                        {shown.map(([k, v]) => <div key={k} class={v.length > 32 ? styles.long : undefined}><dt>{k}</dt><dd>{v}</dd></div>)}
                      </dl>}
                    </div>
                  </li>
                )
              })}
            </ol>
          </section>
        ))}
      </div>}
      {more && <Button class={styles.more} disabled={loading} onClick={onMore}>{loading ? 'Loading…' : 'Show older'}</Button>}
    </div>
  )
}
