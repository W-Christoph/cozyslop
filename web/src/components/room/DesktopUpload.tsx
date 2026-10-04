import { useRoomStore } from './RoomContext'
import styles from './DesktopUpload.module.css'

// Progress and result of the current desktop upload.
export function DesktopUploadStatus() {
  const store = useRoomStore()
  const { state, progress, message } = store.desktopUpload.value
  if (state === 'idle') return null
  return (
    <div class={`${styles.status} ${state === 'error' ? styles.error : ''}`} role="status" aria-live="polite">
      {message}
      {state === 'uploading' && <progress class={styles.progress} value={progress} max={1} />}
      {state === 'uploading' && <button type="button" onClick={() => store.cancelDesktopUpload()}>Cancel</button>}
    </div>
  )
}

// Props for an element that accepts files dropped onto it.
export function useDesktopDrop() {
  const store = useRoomStore()
  if (!store.rights.value.upload) return {}
  return {
    onDragOver: (e: DragEvent) => {
      if (e.dataTransfer?.types.includes('Files')) e.preventDefault()
    },
    onDrop: (e: DragEvent) => {
      if (!e.dataTransfer?.files.length) return
      e.preventDefault()
      void store.uploadToDesktop(Array.from(e.dataTransfer.files))
    },
  }
}
