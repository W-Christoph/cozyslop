import type { DiagSummary } from '../../api'
import { Icon } from '../ui/Icon'
import { Notice } from '../ui/Notice'
import { bytes, when } from './diagFormat'
import styles from './DiagnosticsOverview.module.css'

// How the server has been: since when it runs, whether the run before
// ended well, and what happened in the time shown, counted.
export function DiagnosticsOverview({ summary }: { summary: DiagSummary }) {
  const { counts: c, lastBoot } = summary
  const lastSeen = Number(lastBoot?.attrs?.last_seen) || 0
  const tiles: { label: string; value: number; detail: string; attention: boolean }[] = [
    { label: 'Stalls', value: c.stalls, detail: 'times the server was stuck', attention: c.stalls > 0 },
    { label: 'Errors', value: c.errors, detail: `${c.warnings.toLocaleString()} ${c.warnings === 1 ? 'warning' : 'warnings'}`, attention: c.errors > 0 },
    { label: 'Slow or failed requests', value: c.requests, detail: `${c.failedRequests.toLocaleString()} failed`, attention: c.failedRequests > 0 },
    { label: 'AniList calls', value: c.anilist, detail: `${c.anilistFailed.toLocaleString()} failed`, attention: c.anilistFailed > 0 },
    { label: 'Starts', value: c.boots, detail: c.crashes ? `${c.crashes} after a crash` : 'none after a crash', attention: c.crashes > 0 },
  ]
  return (
    <div class={styles.overview}>
      {lastBoot && (lastBoot.attrs?.previous
        ? <Notice tone="warning">
          Started {when(lastBoot.time)} after a run that did not shut down: it crashed, ran out of memory, or the machine
          stopped.{lastSeen ? ` That run was last seen ${when(lastSeen)}.` : ''}
        </Notice>
        : <Notice>Running since {when(lastBoot.time)}.</Notice>)}
      <dl class={styles.tiles}>
        {tiles.map((t) => (
          <div key={t.label} class={styles.tile}>
            <dt class={styles.label}>
              {t.attention && <span class={styles.attention}><Icon name="alert" size={14} /><span class={styles.hidden}>Needs a look: </span></span>}
              {t.label}
            </dt>
            <dd class={styles.value}>{t.value.toLocaleString()}</dd>
            <dd class={styles.detail}>{t.detail}</dd>
          </div>
        ))}
      </dl>
      <p class={styles.meta}>
        {bytes(summary.sizeBytes)} on disk · entries are kept {summary.retentionDays} days, health samples {summary.sampleDays}
        {summary.dropped > 0 && <> · <span class={styles.warn}>{summary.dropped.toLocaleString()} entries dropped since the server started</span> (more than it could write)</>}
        {summary.tlsHandshakeErrors > 0 && <> · {summary.tlsHandshakeErrors.toLocaleString()} failed TLS handshakes since the server started</>}
      </p>
    </div>
  )
}
