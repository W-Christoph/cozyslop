import Cropper from 'cropperjs'
import { useEffect, useRef, useState } from 'preact/hooks'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { Notice } from '../ui/Notice'
import { useObjectUrl } from './useObjectUrl'
import { cropKeyboard } from '../ui/cropKeyboard'
import cropStyles from '../ui/Cropper.module.css'
import styles from './ScreenshotModal.module.css'

export function ScreenshotModal({ source, onCrop, onClose }: { source: Blob; onCrop: (blob: Blob) => void; onClose: () => void }) {
  const url = useObjectUrl(source)
  const image = useRef<HTMLImageElement>(null)
  const cropper = useRef<Cropper | null>(null)
  const mounted = useRef(true)
  const [ready, setReady] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    mounted.current = true
    if (!url || !image.current) return
    setReady(false)
    cropper.current = new Cropper(image.current, { zoomable: false, autoCropArea: 0.3, ready: () => setReady(true) })
    return () => { mounted.current = false; cropper.current?.destroy(); cropper.current = null }
  }, [url])
  function crop() {
    try {
      const canvas = cropper.current?.getCroppedCanvas()
      if (!canvas) throw new Error('No crop')
      canvas.toBlob((blob) => {
        if (!mounted.current) return
        if (blob) onCrop(blob)
        else setError('Could not crop this screenshot.')
      }, 'image/png')
    } catch { setError('Could not crop this screenshot.') }
  }
  return <Modal title="Crop screenshot" size="xl" onClose={onClose}>
    <p class={styles.hint}>Drag to choose the part of the picture to post in chat.</p>
    <div class={`${cropStyles.frame} ${styles.cropper}`} tabIndex={0} role="group" aria-label="Crop area: arrow keys move, Shift and arrow keys resize"
      onKeyDown={(e) => cropKeyboard(e, cropper.current)}>{url && <img ref={image} src={url} alt="Select an area of the screenshot" onError={() => setError('Could not load this screenshot.')} />}</div>
    <p class={cropStyles.hint}>Arrow keys move the crop; Shift + arrow keys resize it.</p>
    <div class={cropStyles.actions}>
      <Button onClick={onClose}>Close</Button>
      <Button variant="primary" onClick={crop} disabled={!ready}>Crop</Button>
    </div>
    {error && <Notice tone="error">{error}</Notice>}
  </Modal>
}
