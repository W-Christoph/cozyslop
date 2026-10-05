import { createPortal } from 'preact/compat'
import { useId, useState } from 'preact/hooks'
import { useDialogFocus } from '../ui/useDialogFocus'
import type { ChatMessage } from '../../room/protocol'
import styles from './MediaModal.module.css'

// A chat picture or video, large, over the dimmed page and without a frame,
// as in CozyCast. A click anywhere closes it; a click on a picture also
// opens it in a new tab.
export function MediaModal({ message, onClose }: { message: ChatMessage; onClose: () => void }) {
  const dialog = useDialogFocus(useId(), onClose)
  const [container] = useState(() => document.fullscreenElement ?? document.body)
  const image = message.type === 'image'
  return createPortal(<div class={styles.backdrop} role="dialog" aria-modal="true" ref={dialog} tabIndex={-1} aria-label={image ? 'Image' : 'Video'} onClick={onClose}>
    <div class={styles.media}>
      {image
        ? <a aria-label="Open in new tab" href={message.mediaUrl} target="_blank" rel="noopener noreferrer"><img src={message.mediaUrl} alt="Chat image" /></a>
        // The player's own controls must not close the preview.
        : <video tabIndex={0} src={message.mediaUrl} controls autoplay loop playsInline onClick={(e) => e.stopPropagation()} />}
    </div>
  </div>, container)
}
