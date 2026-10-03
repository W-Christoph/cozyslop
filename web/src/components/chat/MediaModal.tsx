import { Modal } from '../Modal'
import type { Media } from './InlineMedia'
import styles from './MediaModal.module.css'

export function MediaModal({ media, onClose }: { media: Media; onClose: () => void }) {
  return <Modal title={media.type === 'image' ? 'Image' : 'Video'} onClose={onClose}>
    <div class={styles.media}>
      {media.type === 'image'
        ? <a href={media.url} target="_blank" rel="noopener noreferrer"><img src={media.url} alt="Chat image" /></a>
        : <video src={media.url} controls autoplay loop playsInline />}
    </div>
    <a class={styles.link} href={media.url} target="_blank" rel="noopener noreferrer">Open in new tab</a>
  </Modal>
}
