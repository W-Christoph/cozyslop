import type { AdminUser } from '../../api'
import { Button } from '../Button'
import styles from './AccountRow.module.css'

export function AccountRow({
  user,
  self,
  busy,
  onUpdate,
  onDelete,
  onReset,
}: {
  user: AdminUser
  self: boolean
  busy: boolean
  onUpdate: (
    username: string,
    change: Partial<Pick<AdminUser, 'verified' | 'admin' | 'disabled'>>,
  ) => void
  onDelete: (username: string) => void
  onReset: (username: string) => void
}) {
  return (
    <tr key={user.username}>
      <td class={styles.avatarCell}>
        <img
          class={styles.avatar}
          src={user.avatarUrl || '/png/default_avatar.png'}
          alt={`${user.username}'s avatar`}
        />
      </td>
      <td class={styles.username}>
        {user.username}
        {self && ' (you)'}
      </td>
      <td class={styles.text}>{user.nickname}</td>
      <td class={styles.text}>
        <span
          class={styles.swatch}
          style={{ backgroundColor: user.nameColor }}
        />
        {user.nameColor}
      </td>
      <td class={styles.center}>
        <Button
          accent={user.verified}
          disabled={busy || self}
          onClick={() => onUpdate(user.username, { verified: !user.verified })}
        >
          {user.verified ? 'verified' : 'Not verified'}
        </Button>
      </td>
      <td class={styles.center}>
        <Button
          accent={user.admin}
          disabled={busy || self}
          onClick={() => onUpdate(user.username, { admin: !user.admin })}
        >
          {user.admin ? 'Remove Admin' : 'Make Admin'}
        </Button>
      </td>
      <td class={styles.center}>
        <Button
          accent={!user.disabled}
          disabled={busy || self}
          onClick={() => onUpdate(user.username, { disabled: !user.disabled })}
        >
          {user.disabled ? 'Enable' : 'Disable'}
        </Button>
      </td>
      <td class={styles.center}>
        <Button
          accent
          disabled={busy || self || user.admin}
          title={user.admin ? 'remove admin first' : undefined}
          onClick={() => onDelete(user.username)}
        >
          {user.admin && !self ? 'remove admin first' : 'Delete'}
        </Button>
      </td>
      <td class={styles.center}>
        <Button disabled={busy || self} onClick={() => onReset(user.username)}>
          Reset password
        </Button>
      </td>
    </tr>
  )
}
