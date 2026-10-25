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
        <RadioCards<Preferences['subtitleCoverDetection']> name="subtitleCoverDetection" label="Subtitle detection" columns={3}
          value={prefs.subtitleCoverDetection} onChange={(subtitleCoverDetection) => updatePreferences({ subtitleCoverDetection })}
          options={[
            { value: 'manual', label: 'Always cover', description: 'Cover the full boxes while enabled.' },
            { value: 'region', label: 'Detect region', description: 'Experimental: cover the text area found inside each box.' },
            { value: 'box', label: 'Detect presence', description: 'Experimental: cover the whole box when text is found.' },
          ]} />
        {prefs.subtitleCoverDetection !== 'manual' && <div>
          <SettingRow title="Experimental detection" description="Looks for bright, outlined lettering inside your boxes. May miss subtitles or mistake scenery for text. Short subtitles can appear before detection. Runs locally; slows down on busy devices. Hover over a box to adjust its search area.">{null}</SettingRow>
          <SettingRow title={`Detection interval: ${prefs.subtitleCoverDetectionInterval} ms`} description="Lower values detect subtitles sooner but use more processing power. Busy devices may check less often.">{null}</SettingRow>
          <Slider label="Detection interval" value={prefs.subtitleCoverDetectionInterval} stops={[25, 100, 175, 250]} step={25} format={(value) => `${value} ms`}
            onChange={(subtitleCoverDetectionInterval) => updatePreferences({ subtitleCoverDetectionInterval })} />
        </div>}
        <ToggleRow title="Two cover boxes" description="Position and resize each box separately. Both follow the toolbar button."
          checked={prefs.subtitleCoverTwoBoxes} onChange={(subtitleCoverTwoBoxes) => updatePreferences({ subtitleCoverTwoBoxes })} />
        <RadioCards<Preferences['subtitleCoverStyle']> name="subtitleCoverStyle" label="Cover look" columns={2} mobileColumns={2}
          value={prefs.subtitleCoverStyle} onChange={(subtitleCoverStyle) => updatePreferences({ subtitleCoverStyle })}
          options={[
            { value: 'blur', label: 'Blur', description: 'The picture shows through, blurred.' },
            { value: 'black', label: 'Black', description: 'Covers the area completely.' },
          ]} />
        {prefs.subtitleCoverStyle === 'blur' && <div>
          <SettingRow title={`Blur: ${prefs.subtitleCoverBlur} px`} description="Stronger hides large subtitles better.">{null}</SettingRow>
          <Slider label="Blur strength" value={prefs.subtitleCoverBlur} stops={BLURS} step={2.5} format={(value) => `${value}`}
            onChange={(subtitleCoverBlur) => updatePreferences({ subtitleCoverBlur })} />
          <SettingRow title={`Feather edges: ${prefs.subtitleCoverFeather} px`} description="Adds a soft fade outside the box. The box you drag stays fully blurred. Zero keeps a sharp edge.">{null}</SettingRow>
          <Slider label="Feather edges" value={prefs.subtitleCoverFeather} stops={[0, 10, 20, 30, 40]} step={2.5} format={(value) => `${value}`}
            onChange={(subtitleCoverFeather) => updatePreferences({ subtitleCoverFeather })} />
        </div>}
      </>}
    </Section>
  )
}
