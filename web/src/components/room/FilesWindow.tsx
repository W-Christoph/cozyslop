import { useRef, useState } from 'preact/hooks'
import { Button, ButtonLink } from '../Button'
import { Modal } from '../Modal'
import { EmptyState } from '../ui/EmptyState'
import { Icon } from '../ui/Icon'
import { useRoomStore } from './RoomContext'
import { fileSize } from './fileSize'
import styles from './FilesWindow.module.css'

// The Downloads folder of the room's desktop: upload into it, see what is in
// it and download from it. For people with the upload right.
export function FilesWindow({ onClose }: { onClose: () => void }) {
  const store = useRoomStore()
  const input = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const files = store.desktopFiles.value
  const { state, progress, message } = store.desktopUpload.value
  const uploading = state === 'uploading'
  const upload = (list: FileList | null | undefined) => { void store.uploadToDesktop(Array.from(list ?? [])) }
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
      <Button accent disabled={uploading} onClick={() => input.current?.click()}>{uploading ? 'Uploading…' : 'Choose files'}</Button>
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
    {files.length === 0
      ? <EmptyState icon="folder" title="Nothing in Downloads yet">Files you upload, and files downloaded on the desktop, show up here.</EmptyState>
      : <ul class={styles.list}>
        {files.map((file) => <li class={styles.file} key={`${file.type}:${file.name}`}>
          <span class={styles.fileIcon}><Icon name={file.type === 'dir' ? 'folder' : 'file'} size={18} /></span>
          <span class={styles.name} title={file.name}>{file.name}</span>
          <span class={styles.size}>{file.type === 'dir' ? 'Folder' : fileSize(file.size)}</span>
          {file.type === 'file'
            ? <ButtonLink size="sm" variant="ghost" icon="download" href={store.desktopFileUrl(file.name)} download={file.name}>Download</ButtonLink>
            : <span />}
        </li>)}
      </ul>}
  </Modal>
}
