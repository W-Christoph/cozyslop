import { useState } from 'preact/hooks'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Notice } from '../ui/Notice'
import { useRoomStore } from './RoomContext'

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
    <Button icon="power" onClick={() => setConfirm(true)}>Restart</Button>
    {store.error.value && <Notice tone="error">{store.error.value}</Notice>}
    {confirm && <Modal compact title="Restart room?" onClose={close} footer={<>
      <Button onClick={close}>Cancel</Button>
      <Button variant="danger" onClick={restart}>Restart</Button>
    </>}>
      <p>The desktop restarts for everyone. The browser reopens its tabs afterwards, but whatever is playing stops and unsaved input in pages is lost.</p>
      <p>Trusted users can restart once per hour per room. Admins can restart at any time.</p>
    </Modal>}
  </>
}
