import { preferences, updatePreferences, type Preferences } from '../../app/state'
import { RadioCards } from '../ui/RadioCards'
import { Section, SettingRow, ToggleRow } from '../ui/Section'
import { Slider } from '../ui/Slider'

const BLURS = [5, 10, 15, 20, 25, 30]

// Options for the subtitle cover (room/SubtitleCover).
export function SubtitleCoverSection() {
  const prefs = preferences.value
  return (
    <Section title="Subtitle cover" description="For learning a language: a box you put over the subtitles, so you listen instead of reading. Only you see it.">
      <ToggleRow title="Subtitle cover button" description="A button on the left of the toolbar shows and hides the box. Drag the box to move it, and its edges or corners to resize it."
        checked={prefs.subtitleCover} onChange={(subtitleCover) => updatePreferences({ subtitleCover })} />
      {prefs.subtitleCover && <>
        <RadioCards<Preferences['subtitleCoverStyle']> name="subtitleCoverStyle" label="Cover look" columns={2} mobileColumns={2}
          value={prefs.subtitleCoverStyle} onChange={(subtitleCoverStyle) => updatePreferences({ subtitleCoverStyle })}
          options={[
            { value: 'blur', label: 'Blur', description: 'The picture shows through, blurred.' },
            { value: 'black', label: 'Black', description: 'Covers the area completely.' },
          ]} />
        {prefs.subtitleCoverStyle === 'blur' && <div>
          <SettingRow title={`Blur: ${prefs.subtitleCoverBlur} px`} description="Stronger hides large subtitles better.">{null}</SettingRow>
          <Slider label="Blur strength" value={prefs.subtitleCoverBlur} stops={BLURS} format={(value) => `${value}`}
            onChange={(subtitleCoverBlur) => updatePreferences({ subtitleCoverBlur })} />
        </div>}
      </>}
    </Section>
  )
}
