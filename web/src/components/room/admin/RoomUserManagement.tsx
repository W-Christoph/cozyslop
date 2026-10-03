import { useId, useState } from 'preact/hooks'
import { PermissionTable } from '../../admin/PermissionTable'
import { Button } from '../../Button'
import { Modal } from '../../Modal'
import { useRoomStore } from '../RoomContext'
import { AnonymousBans } from './AnonymousBans'
import { CurrentRoomUsers } from './CurrentRoomUsers'
import styles from './RoomUserManagement.module.css'

const tabs = ['Current users', 'All users', 'Anonymous bans'] as const

export function RoomUserManagement({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const [tab, setTab] = useState(0)
  const id = useId()
  if (!store.rights.value.admin) return null
  return (
    <Modal title="User Management" onClose={onClose}>
      <div class={styles.content}>
        <div class={styles.tabs} role="tablist" aria-label="User management"
          onKeyDown={(e) => {
            let next = tab
            if (e.key === 'ArrowRight') next = (tab + 1) % tabs.length
            else if (e.key === 'ArrowLeft') next = (tab + tabs.length - 1) % tabs.length
            else if (e.key === 'Home') next = 0
            else if (e.key === 'End') next = tabs.length - 1
            else return
            e.preventDefault()
            setTab(next)
            e.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]')[next]?.focus()
          }}>
          {tabs.map((label, index) => (
            <Button key={label} role="tab" id={`${id}-tab-${index}`}
              aria-selected={tab === index} aria-controls={`${id}-panel`}
              tabIndex={tab === index ? 0 : -1} accent={tab === index}
              onClick={() => setTab(index)}>{label}</Button>
          ))}
        </div>
        <div role="tabpanel" id={`${id}-panel`} aria-labelledby={`${id}-tab-${tab}`} tabIndex={0}>
          {tab === 0 && <CurrentRoomUsers />}
          {tab === 1 && <PermissionTable room={store.room} />}
          {tab === 2 && <AnonymousBans />}
        </div>
      </div>
    </Modal>
  )
}
