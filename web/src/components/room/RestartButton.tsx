import { useState } from 'preact/hooks'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { useRoomStore } from './RoomContext'
import styles from './RestartButton.module.css'

export function RestartButton() {
  const store = useRoomStore()
  const [confirm, setConfirm] = useState(false)
  const { admin, trusted } = store.rights.value
  if (!store.restartAvailable.value || (!admin && !trusted)) return null
  const close = () => setConfirm(false)
  const restart = () => {
    store.restart()
    close()
  }
  return <>
    <Button onClick={() => setConfirm(true)}>Restart</Button>
    {store.error.value && <p role="alert">{store.error.value}</p>}
    {confirm && <Modal compact title="Restart room?" onClose={close}>
      <p>The desktop restarts for everyone. Open tabs and pages in the room's browser will be lost.</p>
      <p>Trusted users can restart once per hour per room. Admins can restart at any time.</p>
      <div class={styles.actions}>
        <Button onClick={close}>Cancel</Button>
        <Button accent onClick={restart}>Restart</Button>
      </div>
    </Modal>}
  </>
}
