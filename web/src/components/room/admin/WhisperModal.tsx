import { Modal } from '../../Modal'

// Placeholder until the whisper port lands.
export function WhisperModal({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Whisper User" onClose={onClose}>
      <p>Coming soon.</p>
    </Modal>
  )
}
