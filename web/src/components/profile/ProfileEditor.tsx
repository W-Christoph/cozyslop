import { useEffect, useState } from 'preact/hooks'
import { HexColorInput, HexColorPicker } from 'react-colorful'
import { api, type Me } from '../../api'
import { me } from '../../app/state'
import { Button } from '../Button'
import { ChatPreview } from '../settings/ChatPreview'
import { Badge } from '../ui/Badge'
import { Field, Input } from '../ui/Field'
import { Notice } from '../ui/Notice'
import { AvatarChooser } from './AvatarChooser'
import styles from './ProfileEditor.module.css'

const DEFAULT_COLOR = '#f90'
// A grey or white name colour makes a dull banner; the accent stands in.
function colourful(hex: string): boolean {
  const value = hex.replace('#', '')
  const full = value.length === 3 ? [...value].map((c) => c + c).join('') : value
  const [r, g, b] = [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16))
  return Math.max(r, g, b) - Math.min(r, g, b) > 40
}

const presets = ['#ff9900', '#f87171', '#facc15', '#4ade80', '#2dd4bf', '#38bdf8', '#818cf8', '#c084fc', '#f472b6', '#ffffff']

export function ProfileEditor() {
  const user = me.value
  const [nickname, setNickname] = useState(user?.nickname ?? ''),
    [color, setColor] = useState(user?.nameColor ?? DEFAULT_COLOR)
  const [blob, setBlob] = useState<Blob | null>(null),
    [preview, setPreview] = useState('')
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(''),
    [message, setMessage] = useState('')
  useEffect(() => {
    if (!blob) {
      setPreview('')
      return
    }
    const url = URL.createObjectURL(blob)
    setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [blob])
  useEffect(() => {
    setNickname(me.value?.nickname ?? '')
    setColor(me.value?.nameColor ?? DEFAULT_COLOR)
    setBlob(null)
  }, [user?.username])
  if (!user) return null
  const changed = nickname !== user.nickname || color.toLowerCase() !== user.nameColor.toLowerCase() || !!blob
  function reset() {
    setNickname(user!.nickname)
    setColor(user!.nameColor)
    setBlob(null)
    setError('')
    setMessage('')
  }
  async function save() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const profile = await api.patch<{ user: Me }>('/api/me', {
        nickname,
        nameColor: color,
      })
      me.value = profile.user
      if (blob) {
        const form = new FormData()
        form.append('avatar', blob, 'avatar.png')
        const result = await api.post<{ user: Me }>('/api/me/avatar', form)
        me.value = result.user
        setBlob(null)
      }
      setMessage('Profile saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function removeAvatar() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const result = await api.del<{ user: Me }>('/api/me/avatar')
      me.value = result.user
      setBlob(null)
      setMessage('Avatar removed.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  const avatar = preview || user.avatarUrl
  return (
    <form
      class={styles.editor}
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <div class={styles.card}>
        <div class={styles.banner} style={colourful(color) ? { '--name-colour': color } : undefined} />
        <div class={styles.identity}>
          <AvatarChooser avatar={avatar} disabled={busy} onCrop={setBlob} />
          <div class={styles.names}>
            <div class={styles.nickname}>{nickname || user.nickname}</div>
            <div class={styles.username}>
              {user.username}
              {user.admin && <Badge tone="accent" icon="shield">Admin</Badge>}
              {user.verified && <Badge tone="success" icon="check">Verified</Badge>}
            </div>
          </div>
          {(user.avatarUrl || blob) && (
            <Button size="sm" variant="ghost" disabled={busy} onClick={blob ? () => setBlob(null) : removeAvatar}>
              {blob ? 'Discard new avatar' : 'Remove avatar'}
            </Button>
          )}
        </div>
      </div>

      <Field label="Nickname" hint="The name others see in rooms. Your username stays the same.">
        <Input
          required
          maxLength={12}
          value={nickname}
          onInput={(e) => setNickname(e.currentTarget.value)}
        />
      </Field>

      <div class={styles.colour}>
        <div class={styles.picker}>
          <span class={styles.label}>Name colour</span>
          <HexColorPicker
            color={color}
            onChange={setColor}
            aria-label="Nickname colour"
          />
          <div class={styles.presets}>
            {presets.map((preset) => (
              <button key={preset} type="button" class={styles.preset} style={{ backgroundColor: preset }}
                aria-label={`Use ${preset}`} title={preset} data-current={color.toLowerCase() === preset || undefined} onClick={() => setColor(preset)} />
            ))}
          </div>
          <div class={styles.hex}>
            <span class={styles.swatch} style={{ backgroundColor: color }} />
            <HexColorInput color={color} onChange={setColor} prefixed aria-label="Nickname colour as hex" />
          </div>
        </div>
        <div class={styles.result}>
          <span class={styles.label}>Preview</span>
          <ChatPreview nickname={nickname || user.nickname} color={color} avatarUrl={avatar || '/png/default_avatar.png'} />
        </div>
      </div>

      {error && <Notice tone="error">{error}</Notice>}
      {message && !changed && <Notice tone="success">{message}</Notice>}
      <div class={styles.actions}>
        <Button variant="ghost" disabled={busy || !changed} onClick={reset}>
          Reset
        </Button>
        <Button accent type="submit" disabled={busy || !changed}>
          {busy ? 'Saving…' : 'Save changes'}
        </Button>
      </div>
    </form>
  )
}
