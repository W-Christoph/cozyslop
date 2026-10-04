import { preferences, updatePreferences } from '../../app/state'
import type { RoomStore } from '../../room/store'
import { RestartButton } from '../room/RestartButton'
import { RadioCards } from '../ui/RadioCards'
import { Section, SettingRow, ToggleRow } from '../ui/Section'
import styles from './RoomSection.module.css'

function ListPreview({ left }: { left: boolean }) {
  return (
    <div class={`${styles.preview} ${left ? styles.left : ''}`} aria-hidden="true">
      <div class={styles.screen} />
      <div class={styles.people}><span /><span /><span /></div>
    </div>
  )
}

// The user list and playback. room is the joined room, if the window was
// opened from one.
export function RoomSection({ room }: { room: RoomStore | null }) {
  const prefs = preferences.value
  const restart = !!room && room.restartAvailable.value && (room.rights.value.trusted || room.rights.value.admin)
  return <>
    <Section title="User list" description="The people watching, shown next to the stream.">
      <RadioCards name="userlist" label="User list position" columns={2} value={prefs.userlistOnLeft ? 'left' : 'bottom'}
        onChange={(value) => updatePreferences({ userlistOnLeft: value === 'left' })}
        options={[
          { value: 'bottom', label: 'Below the stream', preview: <ListPreview left={false} /> },
          { value: 'left', label: 'Left of the stream', preview: <ListPreview left /> },
        ]} />
      <ToggleRow title="Show names" description="Write each nickname under its picture."
        checked={prefs.showUsernames} onChange={(showUsernames) => updatePreferences({ showUsernames })} />
      <ToggleRow title="Small profile pictures" description="Leaves more room for the stream."
        checked={prefs.smallPfp} onChange={(smallPfp) => updatePreferences({ smallPfp })} />
      <ToggleRow title="Show who is muted" description="Mark people who have the sound off, and let others see when you do."
        checked={prefs.showIfMuted} onChange={(showIfMuted) => updatePreferences({ showIfMuted })} />
    </Section>
    <Section title="Playback">
      <ToggleRow title="Sound only" description="Receive the room's sound without the picture. Saves bandwidth, for music or a slow connection."
        checked={prefs.audioOnly} onChange={(audioOnly) => updatePreferences({ audioOnly })} />
    </Section>
    {restart && (
      <Section title="This room">
        <SettingRow title="Restart the room" description="Restarts the desktop for everyone, for when the stream or the browser is stuck.">
          <RestartButton />
        </SettingRow>
      </Section>
    )}
  </>
}
