import { Modal } from '../Modal'
import type { ChatMessage } from '../../room/protocol'
import styles from './MediaModal.module.css'

export function MediaModal({ message, onClose }: { message: ChatMessage; onClose: () => void }) {
  return <Modal title={message.type === 'image' ? 'Image' : 'Video'} onClose={onClose}>
    <div class={styles.media}>
      {message.type === 'image'
        ? <a href={message.mediaUrl} target="_blank" rel="noopener noreferrer"><img src={message.mediaUrl} alt="Chat image" /></a>
        : <video src={message.mediaUrl} controls autoplay loop playsInline />}
    </div>
    <a class={styles.link} href={message.mediaUrl} target="_blank" rel="noopener noreferrer">Open in new tab</a>
  </Modal>
}
