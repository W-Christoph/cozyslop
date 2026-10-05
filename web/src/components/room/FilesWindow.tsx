import { useRef, useState } from 'preact/hooks'
import { Button, ButtonLink } from '../Button'
import { Modal } from '../Modal'
import { EmptyState } from '../ui/EmptyState'
import { Icon } from '../ui/Icon'
import { Notice } from '../ui/Notice'
import { useRoomStore } from './RoomContext'
import { fileSize } from './fileSize'
import styles from './FilesWindow.module.css'

// What VLC on the desktop is offered to play.
const playable = /\.(mp4|m4v|mkv|webm|mov|avi|wmv|flv|mpe?g|ts|m2ts|ogv|3gp|mp3|m4a|aac|flac|ogg|oga|opus|wav|wma)$/i

// The Downloads folder of the room's desktop: upload into it, see what is in
// it, download and delete from it, and play a file on the desktop. For
// people with the upload right; playing needs the remote right too.
export function FilesWindow({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const input = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const [confirming, setConfirming] = useState('') // the file asked about deleting
  const [busy, setBusy] = useState('') // the file being deleted or started
  const [failure, setFailure] = useState('')
  const files = store.desktopFiles.value
  const { state, progress, message } = store.desktopUpload.value
  const uploading = state === 'uploading'
  const upload = (list: FileList | null | undefined) => { void store.uploadToDesktop(Array.from(list ?? [])) }
  const act = async (action: 'delete' | 'play', name: string) => {
    setConfirming('')
    setFailure('')
    setBusy(name)
    const error = await store.desktopFileAction(action, name)
    setBusy('')
    if (error) setFailure(`${name}: ${error}`)
    // Out of the way of what now plays.
    else if (action === 'play') onClose()
  }
  return <Modal title="Files" size="lg" onClose={onClose}>
    <div class={`${styles.drop} ${over ? styles.over : ''}`}
      onDragOver={(e) => { if (e.dataTransfer?.types.includes('Files')) { e.preventDefault(); setOver(true) } }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        setOver(false)
        if (!e.dataTransfer?.files.length) return
        e.preventDefault()
        upload(e.dataTransfer.files)
      }}>
      <div class={styles.dropIcon}><Icon name="upload" size={24} /></div>
      <div class={styles.dropText}>
        <strong>Drop files here to send them to the room</strong>
        <span>They land in the desktop's Downloads folder, ready to open in VLC or the browser.</span>
      </div>
      <Button variant="primary" disabled={uploading} onClick={() => input.current?.click()}>{uploading ? 'Uploading…' : 'Choose files'}</Button>
      <input ref={input} type="file" multiple hidden onChange={(e) => {
        const el = e.currentTarget
        upload(el.files)
        el.value = ''
      }} />
    </div>
    {state !== 'idle' && <div class={`${styles.status} ${state === 'error' ? styles.error : state === 'done' ? styles.done : ''}`} role="status" aria-live="polite">
      <div class={styles.statusLine}>
        <Icon name={state === 'error' ? 'alert' : state === 'done' ? 'check' : 'upload'} size={16} />
        <span>{message}</span>
        {uploading && <Button size="sm" variant="ghost" onClick={() => store.cancelDesktopUpload()}>Cancel</Button>}
      </div>
      {uploading && <progress class={styles.progress} value={progress} max={1} />}
    </div>}
    <div class={styles.header}>
      <h3 class={styles.heading}><Icon name="folder" size={18} />Downloads<span>{files.length === 1 ? '1 item' : `${files.length} items`}</span></h3>
      <Button size="sm" variant="ghost" icon="refresh" onClick={() => store.refreshDesktopFiles()}>Refresh</Button>
    </div>
    {failure && <Notice tone="error" compact class={styles.failure}>{failure}</Notice>}
    {files.length === 0
      ? <EmptyState icon="folder" title="Nothing in Downloads yet">Files you upload, and files downloaded on the desktop, show up here.</EmptyState>
      : <ul class={styles.list}>
        {files.map((file) => <li class={styles.file} key={`${file.type}:${file.name}`}>
          <span class={styles.fileIcon}><Icon name={file.type === 'dir' ? 'folder' : 'file'} size={18} /></span>
          <span class={styles.name} title={file.name}>{file.name}</span>
          <span class={styles.size}>{file.type === 'dir' ? 'Folder' : fileSize(file.size)}</span>
          {file.type !== 'file' ? <span />
            : confirming === file.name ? <span class={styles.actions}>
              <Button size="sm" variant="ghost" onClick={() => setConfirming('')}>Cancel</Button>
              <Button size="sm" variant="danger" onClick={() => { void act('delete', file.name) }}>Delete</Button>
            </span>
            : <span class={styles.actions}>
              {store.rights.value.remote && playable.test(file.name)
                && <Button size="sm" variant="ghost" icon="play" disabled={busy !== ''} onClick={() => { void act('play', file.name) }}>Play</Button>}
              <ButtonLink size="sm" variant="ghost" icon="download" href={store.desktopFileUrl(file.name)} download={file.name}
                title="Download" aria-label={`Download ${file.name}`} />
              <Button size="sm" variant="danger-ghost" icon="trash" disabled={busy !== ''} title="Delete" aria-label={`Delete ${file.name}`}
                onClick={() => setConfirming(file.name)} />
            </span>}
        </li>)}
      </ul>}
  </Modal>
}
