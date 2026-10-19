import { ChatPanel } from '../chat/ChatPanel'
import type { TemporaryLine } from '../chat/useChatEvents'
import { ChatViewBar } from './ChatViewBar'
import styles from './ChatStage.module.css'

// The chat view on wide screens: chat in the stream's place, under its bar.
// The room page puts the people on its left. actions: the bar has the
// buttons on (when the room page has no corner for them).
export function ChatStage({ lines, actions }: { lines: readonly TemporaryLine[]; actions: boolean }) {
  return (
    <section data-chat-stage class={styles.stage} aria-label="Chat">
      <ChatViewBar actions={actions} />
      <ChatPanel lines={lines} wide />
    </section>
  )
}
