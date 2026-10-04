import { useRef } from 'preact/hooks'
import { preferences, resolveTheme, updatePreferences, type Accent, type Theme } from '../../app/state'
import { Icon } from '../ui/Icon'
import { RadioCards } from '../ui/RadioCards'
import { Section, ToggleRow } from '../ui/Section'
import styles from './AppearanceSection.module.css'

type FixedTheme = Exclude<Theme, 'system'>
const themes: { value: FixedTheme; label: string }[] = [
  { value: 'default', label: 'Midnight' },
  { value: 'dark', label: 'Graphite' },
  { value: 'legacy', label: 'Onyx' },
  { value: 'light', label: 'Light' },
]
const accents: [Accent, string][] = [
  ['orange', 'Orange'], ['blurple', 'Blurple'], ['blue', 'Blue'], ['teal', 'Teal'],
  ['green', 'Green'], ['pink', 'Pink'], ['red', 'Red'],
]

// A small picture of the app in one theme.
function ThemePreview({ theme }: { theme: FixedTheme }) {
  return (
    <div class={styles.preview} data-theme={theme} aria-hidden="true">
      <div class={styles.window}>
        <div class={styles.previewNav}><span /><span /><span /></div>
        <div class={styles.previewMain}>
          <span class={styles.previewTitle} />
          <span class={styles.previewLine} />
          <span class={styles.previewButton} />
        </div>
      </div>
    </div>
  )
}

export function AppearanceSection() {
  const { theme, accent } = preferences.value
  const swatches = useRef<HTMLDivElement>(null)
  const shown = resolveTheme(theme)
  // One tab stop for the group; the arrow keys move the choice.
  const move = (e: KeyboardEvent) => {
    const step = e.key === 'ArrowRight' || e.key === 'ArrowDown' ? 1 : e.key === 'ArrowLeft' || e.key === 'ArrowUp' ? -1 : 0
    if (!step) return
    e.preventDefault()
    const next = (accents.findIndex(([value]) => value === accent) + step + accents.length) % accents.length
    updatePreferences({ accent: accents[next][0] })
    swatches.current?.querySelectorAll<HTMLButtonElement>('button')[next]?.focus()
  }
  return <>
    <Section title="Theme" description="Colours of the menus and of the room around the stream.">
      <ToggleRow title="Same as device" description="Light when your device is in light mode, Midnight otherwise."
        checked={theme === 'system'} onChange={(on) => updatePreferences({ theme: on ? 'system' : shown })} />
      <RadioCards<FixedTheme> name="theme" label="Theme" columns={4} mobileColumns={2} value={shown}
        onChange={(theme) => updatePreferences({ theme })}
        options={themes.map((option) => ({ ...option, preview: <ThemePreview theme={option.value} /> }))} />
    </Section>
    <Section title="Accent colour" description="Buttons, highlights and whatever is switched on.">
      <div class={styles.accents} role="radiogroup" aria-label="Accent colour" ref={swatches} onKeyDown={move}>
        {accents.map(([value, label]) => (
          <button key={value} type="button" role="radio" aria-checked={accent === value} aria-label={label} title={label}
            tabIndex={accent === value ? 0 : -1} data-accent={value} class={`${styles.accent} ${accent === value ? styles.chosen : ''}`}
            onClick={() => updatePreferences({ accent: value })}>
            {accent === value && <Icon name="check" size={16} />}
          </button>
        ))}
        <span class={styles.accentName}>{accents.find(([value]) => value === accent)?.[1]}</span>
      </div>
    </Section>
  </>
}
