import { preferences, updatePreferences } from '../../app/state'
import { shortcuts, shortcutLabel } from '../room/shortcuts'
import { Section, SettingRow, ToggleRow } from '../ui/Section'
import styles from './ShortcutsSection.module.css'

export function ShortcutsSection() {
  const enabled = preferences.value.shortcuts
  return <>
    <Section>
      <ToggleRow title="Keyboard shortcuts" description="Single keys that control the room. They pause while you type or hold the remote."
        checked={enabled} onChange={(shortcuts) => updatePreferences({ shortcuts })} />
    </Section>
    <Section title="In a room">
      <div class={`${styles.list} ${enabled ? '' : styles.dimmed}`}>
        {shortcuts.map(({ action, label, keys }) => <SettingRow key={action} title={label}>
          {keys.map((key) => <kbd class={styles.key} key={key}>{shortcutLabel(key)}</kbd>)}
        </SettingRow>)}
      </div>
    </Section>
  </>
}
