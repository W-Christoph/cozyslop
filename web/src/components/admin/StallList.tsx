import { useState } from 'preact/hooks'
import type { DiagEntry } from '../../api'
import { Button } from '../Button'
import { Badge } from '../ui/Badge'
import { Notice } from '../ui/Notice'
import { Spinner } from '../ui/Spinner'
import { duration, value, when } from './diagFormat'
import styles from './StallList.module.css'

type Dump = { state: 'loading' } | { state: 'shown'; text: string } | { state: 'failed'; error: string }

// The times the server was stuck, newest first: why, for how long, and the
// dump kept at the start (goroutines, locks and requests under way).
export function StallList({ stalls, loadDump, open = {} }: {
  stalls: DiagEntry[]
  loadDump: (name: string) => Promise<string>
  open?: Record<string, Dump> // dumps shown from the start (snapshots)
}) {
  const [dumps, setDumps] = useState<Record<string, Dump>>(open)
  if (!stalls.length) return <p class={styles.none}>The server was never stuck in this time.</p>
  async function toggle(name: string) {
    if (dumps[name]) {
      setDumps(({ [name]: _, ...rest }) => rest)
      return
    }
    setDumps((d) => ({ ...d, [name]: { state: 'loading' } }))
    try {
      const text = await loadDump(name)
      setDumps((d) => d[name] ? { ...d, [name]: { state: 'shown', text } } : d)
    } catch (e) {
      const error = e instanceof Error ? e.message : 'Something went wrong.'
      setDumps((d) => d[name] ? { ...d, [name]: { state: 'failed', error } } : d)
    }
  }
  return (
    <ol class={styles.list}>
      {episodes(stalls).map(({ start: s, end }) => {
        const dump = typeof s.attrs?.dump === 'string' ? s.attrs.dump : ''
        const list = (end ?? s).attrs?.reasons
        const reasons = Array.isArray(list) ? list.map(value) : []
        const lasted = typeof end?.attrs?.duration_ms === 'number' ? end.attrs.duration_ms : null
        const shown = dump ? dumps[dump] : undefined
        return (
          <li key={s.id} class={styles.stall}>
            <div class={styles.head}>
              {s.msg === 'server stuck' ? <Badge tone="danger" icon="alert">Stuck</Badge> : <Badge>Dump on request</Badge>}
              <time class={styles.time}>{when(s.time)}</time>
              {s.msg === 'server stuck' && <span class={styles.lasted}>
                {lasted !== null ? `for ${duration(lasted)}` : 'not recorded as well again (stopped or restarted meanwhile?)'}
              </span>}
              {dump && <span class={styles.actions}>
                <Button size="sm" variant="ghost" aria-expanded={!!shown} onClick={() => toggle(dump)}>
                  {shown ? 'Hide dump' : 'Show dump'}
                </Button>
                <a class={styles.open} href={`/api/admin/diagnostics/dumps/${encodeURIComponent(dump)}`} target="_blank" rel="noreferrer">Open in a tab</a>
              </span>}
            </div>
            {reasons.length > 0 && <ul class={styles.reasons}>{reasons.map((r, i) => <li key={i}>{r}</li>)}</ul>}
            {shown?.state === 'loading' && <Spinner label="Loading the dump…" />}
            {shown?.state === 'failed' && <Notice tone="error" compact>{shown.error}</Notice>}
            {shown?.state === 'shown' && <pre class={styles.dump} tabIndex={0} aria-label={`Dump of ${when(s.time)}`}>{shown.text}</pre>}
          </li>
        )
      })}
    </ol>
  )
}

// Each "server stuck" with the "server well again" that ended it (which
// names every reason seen meanwhile), and dumps on request, newest first.
function episodes(stalls: DiagEntry[]) {
  const list: { start: DiagEntry; end?: DiagEntry }[] = []
  for (const e of [...stalls].reverse()) {
    const open = [...list].reverse().find((x) => x.start.msg === 'server stuck' && !x.end)
    if (e.msg === 'server well again') {
      if (open) open.end = e
    } else {
      list.push({ start: e })
    }
  }
  return list.reverse()
}
