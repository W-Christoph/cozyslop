import { preferences, updatePreferences } from '../../app/state'
import type { RoomStore } from '../../room/store'
import { RestartButton } from '../room/RestartButton'
import { RadioCards } from '../ui/RadioCards'
import { Section, SettingRow, ToggleRow } from '../ui/Section'
import { SubtitleCoverSection } from './SubtitleCoverSection'
import { supportsStreamBuffer } from '../../neko/streamBuffer'
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
  const bufferSupported = supportsStreamBuffer()
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
      <ToggleRow title="Outline names over the stream" description="In fullscreen the names sit on the picture. A dark outline keeps them readable on bright and busy scenes."
        checked={prefs.outlineNames} disabled={!prefs.showUsernames} onChange={(outlineNames) => updatePreferences({ outlineNames })} />
      <ToggleRow title="Small profile pictures" description="Leaves more room for the stream."
        checked={prefs.smallPfp} onChange={(smallPfp) => updatePreferences({ smallPfp })} />
    </Section>
    <Section title="Muted, paused and listening" description="Paused counts as muted: the pause icon takes the place of the muted one. Listening is the sound without the picture: a music note.">
      <ToggleRow title="See who is muted, paused or listening" description="An icon on the picture of everyone who has the sound off, the stream paused, or only the sound on."
        checked={prefs.showMuted} onChange={(showMuted) => updatePreferences({ showMuted })} />
      <ToggleRow title="Show others when I am" description="Others see an icon on your picture while you have the sound off, the stream paused, or only the sound on."
        checked={prefs.shareMuted} onChange={(shareMuted) => updatePreferences({ shareMuted })} />
    </Section>
    <Section title="While paused" description="Pausing stops the video and sound, and with them almost all the data the room uses.">
      <ToggleRow title="Chat takes the stream's place" description="The room becomes a chat, with a bar at the top to listen (sound only) or watch. Also with Sound only, and while the room's desktop is offline. Not in fullscreen."
        checked={prefs.chatViewOnPause} onChange={(chatViewOnPause) => updatePreferences({ chatViewOnPause })} />
      <SettingRow title="Preview of the desktop" description="A small, blurry picture of what is on the stream in a corner of the chat, renewed every few seconds. A few kB each time.">{null}</SettingRow>
      <RadioCards name="streamPreview" label="Preview of the desktop" columns={4} mobileColumns={2} disabled={!prefs.chatViewOnPause}
        value={String(prefs.streamPreview)} onChange={(value) => updatePreferences({ streamPreview: Number(value) })}
        options={[
          { value: '0', label: 'Off' },
          { value: '2', label: 'Every 2 s' },
          { value: '5', label: 'Every 5 s' },
          { value: '10', label: 'Every 10 s' },
        ]} />
    </Section>
    <Section title="Playback">
      <SettingRow title="Stream buffer" description={bufferSupported
        ? 'Smooth short network interruptions by keeping some audio and video in reserve. More buffering puts you behind live: controls feel delayed and chat may arrive ahead of the picture. The browser may use less or more than requested. Changes take effect gradually. Only affects you.'
        : 'This browser does not support a custom stream buffer. Playback uses automatic buffering.'}>{null}</SettingRow>
      <RadioCards name="streamBuffer" label="Stream buffer" columns={5} mobileColumns={2} disabled={!bufferSupported}
        value={prefs.streamBuffer === null ? 'auto' : String(prefs.streamBuffer)}
        onChange={(value) => updatePreferences({ streamBuffer: value === 'auto' ? null : Number(value) })}
        options={[
          { value: 'auto', label: 'Auto' },
          { value: '500', label: '0.5 s' },
          { value: '1000', label: '1 s' },
          { value: '2000', label: '2 s' },
          { value: '4000', label: '4 s' },
        ]} />
      <ToggleRow title="Sound only" description="Receive the room's sound without the picture. Saves bandwidth, for music or a slow connection."
        checked={prefs.audioOnly} onChange={(audioOnly) => updatePreferences({ audioOnly })} />
    </Section>
    <Section title="Desktop">
      <ToggleRow title="Ask before pasting into the desktop" description="Preview clipboard text before sharing it with the room."
        checked={prefs.askBeforePaste} onChange={(askBeforePaste) => updatePreferences({ askBeforePaste })} />
    </Section>
    <SubtitleCoverSection />
    {restart && (
      <Section title="This room">
        <SettingRow title="Restart the room" description="Restarts the desktop for everyone, for when the stream or the browser is stuck.">
          <RestartButton />
        </SettingRow>
      </Section>
    )}
  </>
}
