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
    </Section>
    <Section title="Muted and paused" description="Paused counts as muted: the pause icon takes the place of the muted one.">
      <ToggleRow title="See who is muted or paused" description="An icon on the picture of everyone who has the sound off or the stream paused."
        checked={prefs.showMuted} onChange={(showMuted) => updatePreferences({ showMuted })} />
      <ToggleRow title="Show others when I am" description="Others see an icon on your picture while you have the sound off or the stream paused."
        checked={prefs.shareMuted} onChange={(shareMuted) => updatePreferences({ shareMuted })} />
    </Section>
    <Section title="Playback">
      <ToggleRow title="Sound only" description="Receive the room's sound without the picture. Saves bandwidth, for music or a slow connection."
        checked={prefs.audioOnly} onChange={(audioOnly) => updatePreferences({ audioOnly })} />
    </Section>
    <Section title="Desktop">
      <ToggleRow title="Ask before pasting into the desktop" description="Preview clipboard text before sharing it with the room."
        checked={prefs.askBeforePaste} onChange={(askBeforePaste) => updatePreferences({ askBeforePaste })} />
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
