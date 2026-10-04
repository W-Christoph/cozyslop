import { useId, useState } from 'preact/hooks'
import { api, type InviteView } from '../../api'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Field, Input, Select } from '../ui/Field'
import { Notice } from '../ui/Notice'
import { RadioCards } from '../ui/RadioCards'
import { ToggleRow } from '../ui/Section'
import styles from './InviteModal.module.css'

export function InviteModal({
  room,
  onClose,
}: {
  room: string
  onClose: () => void
}) {
  const [temporary, setTemporary] = useState(false),
    [remote, setRemote] = useState(false),
    [image, setImage] = useState(false),
    [upload, setUpload] = useState(false)
  const [name, setName] = useState(''),
    [maxUses, setMaxUses] = useState('1'),
    [expiry, setExpiry] = useState('5')
  const [link, setLink] = useState(''),
    [error, setError] = useState(''),
    [message, setMessage] = useState(''),
    [busy, setBusy] = useState(false)
  const form = useId()
  async function generate() {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      const invite = await api.post<InviteView>('/api/admin/invites', {
        room,
        temporary,
        name,
        remote,
        image,
        upload,
        maxUses: maxUses === '' ? null : Number(maxUses),
        expiresInMinutes: expiry === '' ? null : Number(expiry),
      })
      setLink(location.origin + invite.path)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Something went wrong.')
    } finally {
      setBusy(false)
    }
  }
  async function copy() {
    setError('')
    setMessage('')
    try {
      await navigator.clipboard.writeText(link)
      setMessage('Link copied.')
    } catch {
      setError('Could not copy the link. Select it and copy it manually.')
    }
  }
  const changed = () => { setLink(''); setMessage('') }
  return (
    <Modal title={`Invite to ${room}`} onClose={onClose} footer={<>
      <Button onClick={onClose}>{link ? 'Done' : 'Cancel'}</Button>
      <Button accent type="submit" form={form} disabled={busy}>
        {link ? 'Generate another' : 'Generate link'}
      </Button>
    </>}>
      <form
        id={form}
        class={styles.form}
        onSubmit={(e) => {
          e.preventDefault()
          void generate()
        }}
      >
        <RadioCards name="inviteType" label="Kind of link" columns={2} value={temporary ? 'access' : 'invite'}
          onChange={(value) => { setTemporary(value === 'access'); changed() }}
          options={[
            { value: 'invite', label: 'Invite', description: 'An account that opens it keeps access to the room.' },
            { value: 'access', label: 'Temporary', description: 'Lets anyone in without an account, for that visit.' },
          ]} />
        <div class={styles.group}>
          <div class={styles.groupLabel}>Whoever uses it may</div>
          <ToggleRow title="Use the remote" checked={remote} onChange={(value) => { setRemote(value); changed() }} />
          <ToggleRow title="Post images in chat" checked={image} onChange={(value) => { setImage(value); changed() }} />
          <ToggleRow title="Upload files to the desktop" checked={upload} onChange={(value) => { setUpload(value); changed() }} />
        </div>
        <div class={styles.pair}>
          <Field label="Max uses">
            <Select
              value={maxUses}
              onChange={(e) => { setMaxUses(e.currentTarget.value); changed() }}
            >
              <option value="1">1</option>
              <option value="5">5</option>
              <option value="10">10</option>
              <option value="">Unlimited</option>
            </Select>
          </Field>
          <Field label="Expires after">
            <Select
              value={expiry}
              onChange={(e) => { setExpiry(e.currentTarget.value); changed() }}
            >
              <option value="5">5 minutes</option>
              <option value="60">1 hour</option>
              <option value="1440">1 day</option>
              <option value="">Never</option>
            </Select>
          </Field>
        </div>
        <Field label="Name" hint="Optional. To tell your invites apart later.">
          <Input
            maxLength={64}
            value={name}
            onInput={(e) => { setName(e.currentTarget.value); changed() }}
          />
        </Field>
        {link && (
          <Field label="Link">
            <div class={styles.link}>
              <Input
                class={styles.code}
                readOnly
                value={link}
                onFocus={(e) => e.currentTarget.select()}
              />
              <Button icon="copy" onClick={copy}>Copy</Button>
            </div>
          </Field>
        )}
        {error && <Notice tone="error">{error}</Notice>}
        {message && <Notice tone="success">{message}</Notice>}
      </form>
    </Modal>
  )
}
