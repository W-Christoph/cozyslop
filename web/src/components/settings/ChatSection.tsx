import { useEffect, useRef, useState } from 'preact/hooks'
import { preferences, updatePreferences, type ChatStyle } from '../../app/state'
import { RadioCards } from '../ui/RadioCards'
import { Section, SectionLabel, SettingRow, ToggleRow } from '../ui/Section'
import { Slider } from '../ui/Slider'
import { ChatPreview } from './ChatPreview'
import styles from './ChatSection.module.css'

const SCALES = [80, 90, 100, 110, 120, 130, 140]
const WIDTHS = [100, 110, 120, 130, 140, 150, 160, 170]

// A few lines of chat drawn as bars, for the cards.
function MiniChat({ style, avatars }: { style: ChatStyle; avatars: boolean }) {
  const lines = style === 'compact' ? 3 : 2
  return (
    <div class={styles.mini} data-style={style} aria-hidden="true">
      <div class={styles.miniMessage}>
        {avatars && style !== 'compact' && <span class={styles.miniAvatar} />}
        <div class={styles.miniText}>
          {Array.from({ length: lines }, (_, index) => <div key={index} class={styles.miniLine}>
            {(style === 'compact' || index === 0) && <span class={styles.miniName} />}
            <span class={styles.miniBody} style={{ '--length': `${[70, 45, 60][index]}%` }} />
          </div>)}
        </div>
      </div>
    </div>
  )
}

export function ChatSection() {
  const prefs = preferences.value
  const compact = prefs.chatStyle === 'compact'
  // Pinned at the top (top: -1px leaves a pixel cut off): the controls then
  // scroll under it, which its shadow shows.
  const sticky = useRef<HTMLDivElement>(null)
  const [stuck, setStuck] = useState(false)
  useEffect(() => {
    if (!sticky.current) return
    const observer = new IntersectionObserver(([entry]) => {
      setStuck(entry.intersectionRatio < 1 && entry.intersectionRect.top > entry.boundingClientRect.top)
    }, { threshold: 1 })
    observer.observe(sticky.current)
    return () => observer.disconnect()
  }, [])
  return <>
    <Section title="Look" description="How chat messages look in the room. Every change shows in the preview.">
      <div ref={sticky} class={`${styles.sticky} ${stuck ? styles.stuck : ''}`}>
        <SectionLabel>Preview</SectionLabel>
        <ChatPreview sized />
      </div>
      <div class={styles.group}>
        <SectionLabel>Message display</SectionLabel>
        <RadioCards<ChatStyle> name="chatStyle" label="Message display" columns={3} value={prefs.chatStyle}
          onChange={(chatStyle) => updatePreferences({ chatStyle })}
          options={[
            { value: 'classic', label: 'Classic', description: 'Each person in a bubble.', preview: <MiniChat style="classic" avatars={prefs.chatAvatars} /> },
            { value: 'modern', label: 'Modern', description: 'Flat, with more air.', preview: <MiniChat style="modern" avatars={prefs.chatAvatars} /> },
            { value: 'compact', label: 'Compact', description: 'Time and name on every message.', preview: <MiniChat style="compact" avatars={false} /> },
          ]} />
      </div>
      <div class={styles.group}>
        <SectionLabel>Profile pictures{compact && <span class={styles.hint}> · not in Compact</span>}</SectionLabel>
        <RadioCards name="chatAvatars" label="Profile pictures" columns={2} mobileColumns={2} disabled={compact}
          value={prefs.chatAvatars && !compact ? 'on' : 'off'}
          onChange={(value) => updatePreferences({ chatAvatars: value === 'on' })}
          options={[
            { value: 'on', label: 'Show pictures', description: 'Everyone’s picture beside their messages.', preview: <MiniChat style={compact ? 'classic' : prefs.chatStyle} avatars /> },
            { value: 'off', label: 'Names only', description: 'Just the name, for more text per line.', preview: <MiniChat style={compact ? 'classic' : prefs.chatStyle} avatars={false} /> },
          ]} />
      </div>
      <div>
        <SettingRow title={`Text size: ${prefs.chatScale}%`} description="Of chat messages.">{null}</SettingRow>
        <Slider label="Chat text size" value={prefs.chatScale} stops={SCALES} format={(value) => `${value}%`}
          onChange={(chatScale) => updatePreferences({ chatScale })} />
      </div>
      <div class={styles.width}>
        <SettingRow title={`Chat width: ${prefs.chatWidth}%`} description="How much room the chat takes next to the stream.">{null}</SettingRow>
        <Slider label="Chat width" value={prefs.chatWidth} stops={WIDTHS} format={(value) => `${value}%`}
          onChange={(chatWidth) => updatePreferences({ chatWidth })} />
      </div>
    </Section>
    <Section title="Messages">
      <ToggleRow title="Join and leave messages" description="A line in chat when someone with an account comes or goes."
        checked={prefs.showLeaveJoinMsg} onChange={(showLeaveJoinMsg) => updatePreferences({ showLeaveJoinMsg })} />
      <ToggleRow title="Load pictures and videos on click" description="Nothing is downloaded until you ask for it. For slow connections."
        checked={prefs.manualLoadMedia} onChange={(manualLoadMedia) => updatePreferences({ manualLoadMedia })} />
    </Section>
    <Section title="Fullscreen">
      <ToggleRow title="See-through chat in fullscreen" description="Chat floats over the stream and fades away when nothing is said."
        checked={prefs.transparentChat} onChange={(transparentChat) => updatePreferences({ transparentChat })} />
    </Section>
  </>
}
