import { useEffect, useMemo, useState } from 'preact/hooks'
import { api, type Permission, type RoomInfo } from '../../api'
import { Spinner } from '../ui/Spinner'
import { Notice } from '../ui/Notice'
import { AdminTable } from './AdminTable'
import { blankPermission, PermissionRow } from './PermissionRow'

export function PermissionTable({ room }: { room?: string }) {
  const [permissions, setPermissions] = useState<Permission[]>([]),
    [rooms, setRooms] = useState<RoomInfo[]>([])
  const [error, setError] = useState(''),
    [loading, setLoading] = useState(true)
  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    setPermissions([])
    setRooms([])
    void Promise.allSettled([
      api.get<Permission[]>(
        `/api/admin/permissions${room ? `?room=${encodeURIComponent(room)}` : ''}`,
      ),
      api.get<RoomInfo[]>('/api/rooms'),
    ]).then(([permissionResult, roomResult]) => {
      if (!active) return
      if (permissionResult.status === 'fulfilled')
        setPermissions(permissionResult.value)
      if (roomResult.status === 'fulfilled') setRooms(roomResult.value)
      const failed = [permissionResult, roomResult].find(
        (value) => value.status === 'rejected',
      )
      if (failed?.status === 'rejected')
        setError(
          failed.reason instanceof Error
            ? failed.reason.message
            : 'Something went wrong.',
        )
      setLoading(false)
    })
    return () => {
      active = false
    }
  }, [room])
  const newPermission = useMemo(
    () => blankPermission(room ?? rooms[0]?.name),
    [room, rooms],
  )
  function saved(permission: Permission) {
    setPermissions((list) =>
      [
        ...list.filter(
          (value) =>
            value.room !== permission.room ||
            value.username !== permission.username,
        ),
        permission,
      ].sort(
        (a, b) =>
          a.room.localeCompare(b.room) || a.username.localeCompare(b.username),
      ),
    )
  }
  return (
    <>
      {loading && <Spinner label="Loading permissions…" />}
      {error && <Notice tone="error">{error}</Notice>}
      {!loading && (
        <PermissionRow
          key={`new:${room ?? ''}`}
          permission={newPermission}
          room={room}
          rooms={rooms}
          creating
          onSaved={saved}
          onDeleted={() => {}}
        />
      )}
      <AdminTable
        headings={[
          ...(room ? [] : ['Room']),
          'User',
          'Remote',
          'Images',
          'Upload',
          'Trusted',
          'Invited',
          'Invite name',
          'Banned (until)',
          '',
        ]}
      >
        {permissions.map((permission) => (
          <PermissionRow
            key={JSON.stringify([permission.room, permission.username])}
            permission={permission}
            room={room}
            rooms={rooms}
            onSaved={saved}
            onDeleted={() =>
              setPermissions((list) =>
                list.filter(
                  (value) =>
                    value.room !== permission.room ||
                    value.username !== permission.username,
                ),
              )
            }
          />
        ))}
      </AdminTable>
    </>
  )
}
