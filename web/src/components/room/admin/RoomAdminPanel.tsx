import { useState } from 'preact/hooks'
import { InviteList } from '../../admin/InviteList'
import { InviteModal } from '../../admin/InviteModal'
import { Button } from '../../Button'
import { Modal } from '../../Modal'
import { useRoomStore } from '../RoomContext'
import { RoomAccessSettings } from './RoomAccessSettings'
import { RoomUserManagement } from './RoomUserManagement'
import { StreamSettings } from './StreamSettings'
import { WhisperModal } from './WhisperModal'
import styles from './RoomAdminPanel.module.css'

export function RoomAdminPanel() {
  const store = useRoomStore()
  const [modal, setModal] = useState<'invite' | 'users' | 'invites' | 'whisper' | null>(null)
  const close = () => setModal(null)
  if (!store.rights.value.admin) return null
  return (
    <div class={styles.panel}>
      <RoomAccessSettings />
      <section class={styles.category} aria-label="Admin Tools">
        <h2 class={styles.heading}>Admin Tools</h2>
        <Button onClick={() => setModal('invite')}>Create Invite</Button>
        <Button disabled={!store.remoteHolder.value} onClick={() => store.resetRemote()}>Reset Remote</Button>
        <Button onClick={() => setModal('users')}>User Management</Button>
        <Button onClick={() => setModal('invites')}>Invite Management</Button>
        <Button onClick={() => setModal('whisper')}>Whisper User</Button>
      </section>
      <StreamSettings />
      {modal === 'invite' && <InviteModal room={store.room} onClose={close} />}
      {modal === 'users' && <RoomUserManagement onClose={close} />}
      {modal === 'invites' && <Modal title="Invites" onClose={close}><InviteList room={store.room} /></Modal>}
      {modal === 'whisper' && <WhisperModal onClose={close} />}
    </div>
  )
}
