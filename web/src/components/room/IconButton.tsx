import type { JSX } from 'preact'
import { preferences } from '../../app/state'
import { RoomTooltip } from './RoomTooltip'
import { shortcutKeys, type ShortcutAction } from './shortcuts'
import styles from './IconButton.module.css'

type Props = JSX.IntrinsicElements['button'] & { icon: string; label: string; active?: boolean; badge?: number; shortcut?: ShortcutAction; tooltip?: string | false }
export function IconButton({ icon, label, active = false, badge = 0, shortcut, tooltip, title: _title, class: className, ...props }: Props) {
  const button = <button type="button" {...props} aria-label={label} aria-keyshortcuts={shortcut && preferences.value.shortcuts ? shortcutKeys(shortcut).join(' ') : undefined}
    class={`${styles.button} ${active ? styles.active : ''} ${className ?? ''}`}>
    <img src={`/svg/${icon}.svg`} alt="" />
    {badge > 0 && <span class={styles.badge} aria-hidden="true">{badge > 99 ? '99+' : badge}</span>}
  </button>
  return tooltip === false ? button : <RoomTooltip label={tooltip ?? label} shortcut={shortcut}>{button}</RoomTooltip>
}
