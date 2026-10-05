import { preferences, updatePreferences, type ChatStyle, type ChatWidth } from '../../app/state'
import { Select } from '../ui/Field'
import { RadioCards } from '../ui/RadioCards'
import { Section, SectionLabel, SettingRow, ToggleRow } from '../ui/Section'
import { Slider } from '../ui/Slider'
import { ChatPreview } from './ChatPreview'

const SCALES = [80, 90, 100, 110, 120, 130, 140]

export function ChatSection() {
  const prefs = preferences.value
  return <>
    <Section>
      <SectionLabel>Preview</SectionLabel>
      <ChatPreview />
    </Section>
    <Section title="Messages">
      <RadioCards<ChatStyle> name="chatStyle" label="Message display" value={prefs.chatStyle}
        onChange={(chatStyle) => updatePreferences({ chatStyle })}
        options={[
          { value: 'classic', label: 'Classic', description: 'Each person in a bubble.' },
          { value: 'modern', label: 'Modern', description: 'Flat, with more air.' },
          { value: 'compact', label: 'Compact', description: 'Time and name on every message.' },
        ]} />
      <ToggleRow title="Profile pictures" description={prefs.chatStyle === 'compact' ? 'Not shown in the compact display.' : "Show everyone's picture next to their messages."}
        checked={prefs.chatAvatars} disabled={prefs.chatStyle === 'compact'} onChange={(chatAvatars) => updatePreferences({ chatAvatars })} />
      <div>
        <SettingRow title={`Text size: ${prefs.chatScale}%`} description="Of chat messages.">{null}</SettingRow>
        <Slider label="Chat text size" value={prefs.chatScale} stops={SCALES} format={(value) => `${value}%`}
          onChange={(chatScale) => updatePreferences({ chatScale })} />
      </div>
      <ToggleRow title="Join and leave messages" description="A line in chat when someone with an account comes or goes."
        checked={prefs.showLeaveJoinMsg} onChange={(showLeaveJoinMsg) => updatePreferences({ showLeaveJoinMsg })} />
      <ToggleRow title="Load pictures and videos on click" description="Nothing is downloaded until you ask for it. For slow connections."
        checked={prefs.manualLoadMedia} onChange={(manualLoadMedia) => updatePreferences({ manualLoadMedia })} />
    </Section>
    <Section title="Layout">
      <SettingRow title="Chat width" description="How much room the sidebar takes next to the stream." htmlFor="chat-width">
        <Select id="chat-width" value={prefs.chatWidth} onChange={(e) => updatePreferences({ chatWidth: e.currentTarget.value as ChatWidth })}>
          <option value="default">Default</option>
          <option value="wide">Wide</option>
          <option value="wider">Extra wide</option>
        </Select>
      </SettingRow>
      <ToggleRow title="See-through chat in fullscreen" description="Chat floats over the stream and fades away when nothing is said."
        checked={prefs.transparentChat} onChange={(transparentChat) => updatePreferences({ transparentChat })} />
    </Section>
  </>
}
