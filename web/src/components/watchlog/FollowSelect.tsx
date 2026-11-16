import type { WatchMode } from '../../api'
import styles from './FollowSelect.module.css'

// How someone follows a show, to choose: the others wait for who is
// following; not for who watches casually (along when they are there) or
// has dropped it. What they watched stays either way.
export function FollowSelect({ mode, who, started, onChange }: {
  mode: WatchMode
  who: string // "you", or their nickname
  started: boolean // they have watched some of it
  onChange: (mode: WatchMode) => void
}) {
  return (
    <select class={styles.select} value={mode} aria-label={`How ${who} follow${who === 'you' ? '' : 's'} this show`}
      title={followAbout(mode, who)} onChange={(e) => onChange(e.currentTarget.value as WatchMode)}>
      <option value="following">Following</option>
      <option value="casual">Watching casually</option>
      <option value="dropped">{started ? 'Dropped' : 'Not interested'}</option>
    </select>
  )
}

// What a mode means for the others.
export function followAbout(mode: WatchMode, who: string) {
  const they = who === 'you' ? 'you' : 'they'
  return mode === 'following' ? `The others wait for ${who} before they watch on.`
    : mode === 'casual' ? `The others do not wait for ${who}. What ${they} see when ${they} are there is still counted.`
    : `The others do not wait for ${who}. What was watched stays.`
}
