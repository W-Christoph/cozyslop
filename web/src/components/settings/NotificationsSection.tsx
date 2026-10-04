import { preferences, updatePreferences } from '../../app/state'
import { Section, ToggleRow } from '../ui/Section'

export function NotificationsSection() {
  const prefs = preferences.value
  return <>
    <Section title="Sound">
      <ToggleRow title="Message sound" description="Play a sound for new messages while CozyCast is in the background. Mentions of your name always play it."
        checked={!prefs.muteChatNotification} onChange={(on) => updatePreferences({ muteChatNotification: !on })} />
    </Section>
    <Section title="Browser tab">
      <ToggleRow title="CozyCast first in the tab title" description={'"CozyCast: What is playing" instead of "What is playing - CozyCast".'}
        checked={prefs.titleNameInFront} onChange={(titleNameInFront) => updatePreferences({ titleNameInFront })} />
    </Section>
  </>
}
