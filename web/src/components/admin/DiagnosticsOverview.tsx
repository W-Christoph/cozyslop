import type { ComponentChildren } from 'preact'
import type { DiagSummary } from '../../api'
import { Icon } from '../ui/Icon'
import { Notice } from '../ui/Notice'
import { bytes, plural, when } from './diagFormat'
import styles from './DiagnosticsOverview.module.css'

export type Tile = 'starts' | 'stalls' | 'errors' | 'requests' | 'anilist'

// How the server has been: since when it runs, how the run before ended,
// and what happened in the time shown, counted. A count leads to its
// entries (onPick).
export function DiagnosticsOverview({ summary, onPick, actions }: {
  summary: DiagSummary
  onPick: (tile: Tile) => void
  actions?: ComponentChildren // beside the size line
}) {
  const { counts: c, lastBoot } = summary
  const tiles: { id: Tile; label: string; value: string; detail: string; alarm: boolean }[] = [
    { id: 'starts', label: 'Starts', value: c.boots.toLocaleString(), detail: c.crashes ? `${c.crashes} after the server went down` : 'none after going down', alarm: c.crashes > 0 },
    { id: 'stalls', label: 'Stalls', value: c.stalls.toLocaleString(), detail: c.stalls === 1 ? 'time the server was stuck' : 'times the server was stuck', alarm: c.stalls > 0 },
    { id: 'errors', label: 'Errors', value: c.errors.toLocaleString(), detail: plural(c.warnings, 'warning', 'warnings'), alarm: false },
    { id: 'requests', label: 'Slow or failed requests', value: c.requests.toLocaleString(), detail: `${c.failedRequests.toLocaleString()} failed`, alarm: false },
    {
      id: 'anilist', label: 'AniList, busiest minute', value: `${c.anilistPeak} of 90`, alarm: false,
      detail: `${plural(c.anilist, 'call', 'calls')} · ${bytes(c.anilistSentBytes + c.anilistReceivedBytes)}${c.anilistFailed ? ` · ${c.anilistFailed} failed` : ''}`,
    },
  ]
  return (
    <div class={styles.overview}>
      {lastBoot && <BootNotice summary={summary} />}
      <div class={styles.tiles}>
        {tiles.map((t) => (
          <button key={t.id} type="button" class={styles.tile} onClick={() => onPick(t.id)}>
            <span class={styles.label}>
              {t.alarm && <span class={styles.alarm}><Icon name="alert" size={14} /><span class={styles.hidden}>Needs a look: </span></span>}
              {t.label}
            </span>
            <span class={styles.value}>{t.value}</span>
            <span class={styles.detail}>{t.detail}</span>
          </button>
        ))}
      </div>
      <div class={styles.foot}>
        <p class={styles.meta} title={summary.tlsHandshakeErrors ? `${summary.tlsHandshakeErrors.toLocaleString()} failed TLS handshakes since the server started (mostly scanners)` : undefined}>
          {bytes(summary.sizeBytes)} on disk · entries are kept {summary.retentionDays} days, health samples {summary.sampleDays} days
          {summary.dropped > 0 && <> · <span class={styles.warn}>{plural(summary.dropped, 'entry', 'entries')} dropped since the server started</span> (more than it could write)</>}
        </p>
        {actions}
      </div>
    </div>
  )
}

// Since when the server runs; when the run before did not end cleanly,
// how it ended and where to look next.
function BootNotice({ summary }: { summary: DiagSummary }) {
  const boot = summary.lastBoot!
  if (!boot.attrs?.previous) return <Notice>Running since {when(boot.time)}.</Notice>
  const lastSeen = Number(boot.attrs.last_seen) || 0
  const crash = typeof boot.attrs.crash === 'string' ? boot.attrs.crash : ''
  return (
    <Notice tone="warning">
      Started {when(boot.time)} after the server went down{lastSeen ? `; it was last seen ${when(lastSeen)}` : ''}.{' '}
      {crash ? <>Go ended it: <code>{crash}</code> (the whole message is under What went wrong).</>
        : <>Go did not end it, so something did from outside: most likely the machine ran out of memory (on the
          machine, <code>journalctl -k | grep -i oom</code> says), or it lost power, or the container was killed.</>}
    </Notice>
  )
}
