import { ChatPanel } from '../chat/ChatPanel'
import type { TemporaryLine } from '../chat/useChatEvents'
import { ChatViewBar } from './ChatViewBar'
import styles from './ChatStage.module.css'

// The chat view on wide screens: chat in the stream's place, under its bar.
// The room page puts the people beside it, where the chat was.
export function ChatStage({ lines }: { lines: readonly TemporaryLine[] }) {
  return (
    <section data-chat-stage class={styles.stage} aria-label="Chat">
      <ChatViewBar />
      <ChatPanel lines={lines} wide />
    </section>
  )
}
