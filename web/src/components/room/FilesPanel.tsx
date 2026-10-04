import { useRef, useState } from 'preact/hooks'
import { Button } from '../Button'
import { useRoomStore } from './RoomContext'
import styles from './FilesPanel.module.css'

function fileSize(bytes: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit++ }
  return `${unit === 0 || value >= 100 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`
}

// The Downloads folder of the room's desktop: upload into it, see what is in
// it and download from it. For people with the upload right.
export function FilesPanel() {
  const store = useRoomStore()
  const input = useRef<HTMLInputElement>(null)
  const [over, setOver] = useState(false)
  const files = store.desktopFiles.value
  const uploading = store.desktopUpload.value.state === 'uploading'
  const upload = (list: FileList | null | undefined) => { void store.uploadToDesktop(Array.from(list ?? [])) }
  return <section class={styles.panel} aria-label="Files">
    <h2 class={styles.title}>Files</h2>
    <p class={styles.hint}>The Downloads folder of the room's desktop. Uploads land there, for example to open them in VLC.</p>
    <div class={`${styles.drop} ${over ? styles.over : ''}`}
      onDragOver={(e) => { if (e.dataTransfer?.types.includes('Files')) { e.preventDefault(); setOver(true) } }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        setOver(false)
        if (!e.dataTransfer?.files.length) return
        e.preventDefault()
        upload(e.dataTransfer.files)
      }}>
      <Button accent disabled={uploading} onClick={() => input.current?.click()}>{uploading ? 'Uploading…' : 'Upload files'}</Button>
      <span>or drop them here</span>
      <input ref={input} type="file" multiple hidden onChange={(e) => {
        const el = e.currentTarget
        upload(el.files)
        el.value = ''
      }} />
    </div>
    <div class={styles.header}>
      <span>{files.length === 1 ? '1 item' : `${files.length} items`}</span>
      <Button onClick={() => store.refreshDesktopFiles()}>Refresh</Button>
    </div>
    {files.length === 0
      ? <p class={styles.empty}>Nothing in Downloads yet.</p>
      : <ul class={styles.list}>
        {files.map((file) => <li class={styles.file} key={`${file.type}:${file.name}`}>
          <span class={styles.name} title={file.name}>{file.name}</span>
          <span class={styles.size}>{file.type === 'dir' ? 'Folder' : fileSize(file.size)}</span>
          {file.type === 'file' && <a class={styles.download} href={store.desktopFileUrl(file.name)} download={file.name}>Download</a>}
        </li>)}
      </ul>}
  </section>
}
