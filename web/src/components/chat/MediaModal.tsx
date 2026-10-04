import { useEffect } from 'preact/hooks'
import type { ChatMessage } from '../../room/protocol'
import styles from './MediaModal.module.css'

// A chat picture or video, large, over the dimmed page and without a frame,
// as in CozyCast. A click anywhere closes it; a click on a picture also
// opens it in a new tab.
export function MediaModal({ message, onClose }: { message: ChatMessage; onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])
  const image = message.type === 'image'
  return <div class={styles.backdrop} role="dialog" aria-modal="true" aria-label={image ? 'Image' : 'Video'} onClick={onClose}>
    <div class={styles.media}>
      {image
        ? <a href={message.mediaUrl} target="_blank" rel="noopener noreferrer"><img src={message.mediaUrl} alt="Chat image" /></a>
        // The player's own controls must not close the preview.
        : <video src={message.mediaUrl} controls autoplay loop playsInline onClick={(e) => e.stopPropagation()} />}
    </div>
  </div>
}
