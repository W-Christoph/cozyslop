import { useState } from 'preact/hooks'
import { dumpUrl, type DiagEntry } from '../../api'
import { Button } from '../Button'
import { Badge } from '../ui/Badge'
import { Notice } from '../ui/Notice'
import { Spinner } from '../ui/Spinner'
import { duration, reason, span, value, when } from './diagFormat'
import styles from './Incidents.module.css'

type Dump = { state: 'loading' } | { state: 'shown'; text: string } | { state: 'failed'; error: string }

interface Incident {
  time: number
  kind: 'stuck' | 'down' | 'dump'
  entry: DiagEntry
  until?: number // when it ended: well again, or started again
  lasted?: number // ms, for a stall that ended
  reasons: string[]
  dump: string
  note: string
}

// What went wrong, newest first: the times the server was stuck (why, for
// how long), the times it went down (until it started again, and how it
// ended), and dumps kept on request. Each can show the entries around it.
export function Incidents({ stalls, boots, loadDump, onAround, open = {} }: {
  stalls: DiagEntry[] // newest first
  boots: DiagEntry[] // newest first
  loadDump: (name: string) => Promise<string>
  onAround: (time: number) => void
  open?: Record<string, Dump> // dumps shown from the start (snapshots)
}) {
  const [dumps, setDumps] = useState<Record<string, Dump>>(open)
  const list = incidents(stalls, boots)
  if (!list.length) return <p class={styles.none}>Nothing went wrong in this time: the server was never stuck or down.</p>
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
      {list.map((i) => {
        const shown = i.dump ? dumps[i.dump] : undefined
        return (
          <li key={`${i.kind}${i.entry.id}`} class={styles.incident}>
            <div class={styles.head}>
              {i.kind === 'stuck' ? <Badge tone="danger" icon="alert">Stuck</Badge>
                : i.kind === 'down' ? <Badge tone="danger" icon="power">Down</Badge>
                : <Badge>Dump on request</Badge>}
              <span class={styles.when}>
                {i.kind === 'down'
                  ? <>{i.until ? span(i.time, i.until) : when(i.time)}{i.until ? <span class={styles.lasted}> · {duration(i.until - i.time)}</span> : null}</>
                  : <>{when(i.time)}{i.lasted !== undefined && <span class={styles.lasted}> · for {duration(i.lasted)}</span>}</>}
              </span>
              <span class={styles.actions}>
                <Button size="sm" variant="ghost" onClick={() => onAround(i.time)}>Show what happened around then</Button>
                {i.dump && <Button size="sm" variant="ghost" aria-expanded={!!shown} onClick={() => toggle(i.dump)}>
                  {shown ? 'Hide dump' : 'Show dump'}
                </Button>}
                {i.dump && <a class={styles.open} href={dumpUrl(i.dump)} target="_blank" rel="noreferrer">Open in a tab</a>}
              </span>
            </div>
            {i.note && <p class={styles.note}>{i.note}</p>}
            {i.reasons.length > 0 && <ul class={styles.reasons}>{i.reasons.map((r, n) => <li key={n}>{r}</li>)}</ul>}
            {shown?.state === 'loading' && <Spinner label="Loading the dump…" />}
            {shown?.state === 'failed' && <Notice tone="error" compact>{shown.error}</Notice>}
            {shown?.state === 'shown' && <pre class={styles.dump} tabIndex={0} aria-label={`Dump of ${when(i.time)}`}>{shown.text}</pre>}
          </li>
        )
      })}
    </ol>
  )
}

const list = (e?: DiagEntry) => Array.isArray(e?.attrs?.reasons) ? e.attrs.reasons.map((r) => reason(value(r))) : []
const text = (e: DiagEntry, key: string) => typeof e.attrs?.[key] === 'string' ? e.attrs[key] as string : ''

function incidents(stalls: DiagEntry[], boots: DiagEntry[]): Incident[] {
  const out: Incident[] = []
  // Oldest first, so that "well again" finds its stall.
  const open: Incident[] = []
  for (const e of [...stalls].reverse()) {
    if (e.msg === 'server well again') {
      const i = open.pop()
      if (i) {
        i.until = e.time
        i.lasted = typeof e.attrs?.duration_ms === 'number' ? e.attrs.duration_ms : e.time - i.time
        i.reasons = list(e) // every reason seen meanwhile
      }
      continue
    }
    const i: Incident = { time: e.time, kind: e.msg === 'server stuck' ? 'stuck' : 'dump', entry: e, reasons: list(e), dump: text(e, 'dump'), note: '' }
    if (i.kind === 'stuck') open.push(i)
    out.push(i)
  }
  for (const i of open) {
    const restart = boots.filter((b) => b.time > i.time).at(-1)
    i.note = restart ? `It never got well again: the server stopped while stuck, and started again ${when(restart.time)}.`
      : 'Still stuck, or not checked since.'
  }
  for (const b of boots) {
    if (!b.attrs?.previous) continue
    const lastSeen = Number(b.attrs.last_seen) || b.time
    const crash = text(b, 'crash')
    out.push({
      time: lastSeen, until: b.time, kind: 'down', entry: b, reasons: [], dump: text(b, 'dump'),
      note: crash ? `Go ended it: ${crash}`
        : 'Go did not end it, so something did from outside: most likely the machine ran out of memory (journalctl -k | grep -i oom on the machine says), or it lost power, or the container was killed.',
    })
  }
  return out.sort((a, b) => b.time - a.time)
}
