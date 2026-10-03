import { useEffect, useState } from 'preact/hooks'
import { HexColorPicker } from 'react-colorful'
import { api, type Me } from '../../api'
import { me } from '../../app/state'
import { Button } from '../Button'
import { AvatarChooser } from './AvatarChooser'
import styles from './ProfileEditor.module.css'

export function ProfileEditor({ onSaved }: { onSaved?: () => void }) {
  const user = me.value
  const [nickname, setNickname] = useState(user?.nickname ?? ''),
    [color, setColor] = useState(user?.nameColor ?? '#f90')
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
    setColor(me.value?.nameColor ?? '#f90')
    setBlob(null)
  }, [user?.username])
  if (!user)
    return (
      <p>
        Please <a href="/login">log in</a> to edit your profile.
      </p>
    )
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
      setMessage('Profile edited!')
      onSaved?.()
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
      setMessage('Avatar removed!')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  return (
    <form
      class={styles.editor}
      onSubmit={(e) => {
        e.preventDefault()
        void save()
      }}
    >
      <div class={styles.title}>Profile</div>
      <div class={styles.dataEdit}>
        <div class={styles.upperData}>
          <div class={styles.dataContainer}>
            <div class={styles.textInfoContainer}>
              <div class={styles.textContainer}>
                <div class={styles.textInfo}>Username</div>
                <div class={styles.textData}>{user.username}</div>
              </div>
              <div class={styles.textContainer}>
                <div class={styles.textInfo}>Nickname</div>
                <div class={styles.textData}>{nickname}</div>
              </div>
            </div>
            <div class={styles.avatarContainer}>
              <AvatarChooser
                avatar={preview || user.avatarUrl}
                disabled={busy}
                onCrop={setBlob}
              />
              <Button disabled={busy} onClick={removeAvatar}>
                Remove avatar
              </Button>
            </div>
          </div>
          <label class={styles.nickname}>
            Edit Nickname
            <input
              required
              maxLength={12}
              value={nickname}
              onInput={(e) => setNickname(e.currentTarget.value)}
            />
          </label>
        </div>
        <div class={styles.colorData}>
          <div class={styles.picker}>
            <HexColorPicker
              color={color}
              onChange={setColor}
              aria-label="Nickname colour"
            />
          </div>
          <div class={styles.previews}>
            {(['legacy', 'default'] as const).map((theme) => (
              <div key={theme} class={styles.preview} data-theme={theme}>
                <div class={styles.message}>
                  <div class={styles.username} style={{ color }}>
                    {nickname}
                    <span class={styles.timestamp}> 8:00 AM</span>
                  </div>
                  <div class={styles.chatText}>
                    This is how it will look with the {theme} theme
                    <br />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
      <Button type="submit" disabled={busy}>
        Save
      </Button>
      {error && <p role="alert">{error}</p>}
      {message && <p role="status">{message}</p>}
    </form>
  )
}
