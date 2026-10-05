import type { RefObject } from 'preact'
import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useRoomStore } from '../room/RoomContext'
import { RoomTooltip } from '../room/RoomTooltip'
import { ConfirmUpload } from './ConfirmUpload'
import { ScreenshotModal } from './ScreenshotModal'
import styles from './UploadControls.module.css'

const accepted = ['image/png', 'image/jpeg', 'image/gif', 'image/webp', 'video/mp4', 'video/webm']

export function UploadControls({ visible, receiveFile }: {
  visible: boolean
  receiveFile: RefObject<((file: File) => void) | null>
}) {
  const store = useRoomStore()
  const chooser = useRef<HTMLInputElement>(null)
  const controls = useRef<HTMLDivElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [screenshotFile, setScreenshotFile] = useState(false)
  const [screenshot, setScreenshot] = useState<Blob | null>(null)
  const [error, setError] = useState('')
  const mounted = useRef(true)
  const capturing = useRef(false)
  function select(file: File, fromScreenshot = false) {
    if (!store.rights.peek().image) return
    setError('')
    if (!accepted.includes(file.type)) { setError('Use a PNG, JPEG, GIF, WebP, MP4 or WebM file.'); return }
    setFile(file)
    setScreenshotFile(fromScreenshot)
  }
  useLayoutEffect(() => {
    mounted.current = true
    receiveFile.current = select
    return () => { mounted.current = false; receiveFile.current = null }
  }, [receiveFile, store])
  function capture() {
    if (capturing.current) return
    setError('')
    // The room's stream video is the first video in the room page, before
    // the sidebar; inline chat previews have no srcObject.
    const video = [...(controls.current?.closest('aside')?.parentElement?.querySelectorAll('video') ?? [])]
      .find((element) => element.srcObject !== null)
    if (!video || !video.videoWidth || !video.videoHeight || video.readyState < 2) {
      setError('The video is not ready for a screenshot.')
      return
    }
    capturing.current = true
    try {
      const canvas = document.createElement('canvas')
      canvas.width = video.videoWidth
      canvas.height = video.videoHeight
      const context = canvas.getContext('2d')
      if (!context) throw new Error('No canvas')
      context.drawImage(video, 0, 0, canvas.width, canvas.height)
      canvas.toBlob((blob) => {
        capturing.current = false
        if (!mounted.current || !store.rights.peek().image) return
        if (blob) setScreenshot(blob)
        else setError('Could not capture the video frame.')
      }, 'image/png')
    } catch {
      capturing.current = false
      setError('Could not capture the video frame.')
    }
  }
  return <>
    <div class={styles.controls} ref={controls} hidden={!visible}>
      <input ref={chooser} type="file" aria-label="Choose chat media" class={styles.file} accept={accepted.join(',')}
        onChange={(e) => { const chosen = e.currentTarget.files?.[0]; e.currentTarget.value = ''; if (chosen) select(chosen) }} />
      <RoomTooltip label="Screenshot video"><button type="button" aria-label="Screenshot video" onClick={capture}><img src="/svg/screen_shot.svg" alt="" /></button></RoomTooltip>
      <RoomTooltip label="Upload image or video"><button type="button" aria-label="Upload image or video" onClick={() => chooser.current?.click()}><img src="/svg/image.svg" alt="" /></button></RoomTooltip>
    </div>
    {error && <div class={styles.error} role="alert">{error}</div>}
    {screenshot && <ScreenshotModal source={screenshot} onClose={() => setScreenshot(null)} onCrop={(blob) => {
      select(new File([blob], 'screenshot.png', { type: 'image/png' }), true)
    }} />}
    {file && <ConfirmUpload key={`${file.name}:${file.lastModified}`} file={file} screenshot={screenshotFile} onClose={() => setFile(null)} onUploaded={() => {
      setFile(null)
      if (screenshotFile) setScreenshot(null)
    }} onError={setError} />}
  </>
}
