import { Modal } from '../../Modal'

// Placeholder until the user management port lands (current users with
// permissions, bans and kicks, all users, anonymous bans).
export function RoomUserManagement({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="User Management" onClose={onClose}>
      <p>Coming soon.</p>
    </Modal>
  )
}
