import type { AdminUser } from '../../api'
import { Button } from '../Button'
import { Avatar } from '../ui/Avatar'
import { Badge } from '../ui/Badge'
import { Switch } from '../ui/Switch'
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
    <tr key={user.username} class={user.disabled ? styles.disabled : undefined}>
      <td>
        <div class={styles.account}>
          <Avatar src={user.avatarUrl} size={40} alt={`${user.username}'s avatar`} />
          <div class={styles.names}>
            <div class={styles.username}>
              {user.username}
              {self && <Badge>You</Badge>}
              {user.disabled && <Badge tone="danger">Disabled</Badge>}
            </div>
            <div class={styles.nickname}>
              <span class={styles.swatch} style={{ backgroundColor: user.nameColor }} title={user.nameColor} />
              {user.nickname}
            </div>
          </div>
        </div>
      </td>
      <td class={styles.toggle} data-label="Verified">
        <Switch
          label={`Verified: ${user.username}`}
          checked={user.verified}
          disabled={busy}
          onChange={(verified) => onUpdate(user.username, { verified })}
        />
      </td>
      <td class={styles.toggle} data-label="Admin">
        <Switch
          label={`Admin: ${user.username}`}
          checked={user.admin}
          disabled={busy || self}
          onChange={(admin) => onUpdate(user.username, { admin })}
        />
      </td>
      <td class={styles.toggle} data-label="Enabled">
        <Switch
          label={`Enabled: ${user.username}`}
          checked={!user.disabled}
          disabled={busy || self}
          onChange={(enabled) => onUpdate(user.username, { disabled: !enabled })}
        />
      </td>
      <td>
        <div class={styles.actions}>
          <Button
            size="sm"
            variant="ghost"
            icon="key"
            aria-label={`Reset password of ${user.username}`}
            title="Reset password"
            disabled={busy || self}
            onClick={() => onReset(user.username)}
          />
          <Button
            size="sm"
            variant="danger-ghost"
            icon="trash"

            aria-label={`Delete ${user.username}`}
            title={user.admin ? 'Remove admin first' : 'Delete'}
            disabled={busy || self || user.admin}
            onClick={() => onDelete(user.username)}
          />
        </div>
      </td>
    </tr>
  )
}
