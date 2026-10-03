import Cropper from 'cropperjs'
import { useEffect, useRef, useState } from 'preact/hooks'
import { Button } from '../Button'
import { Modal } from '../Modal'
import { useObjectUrl } from './useObjectUrl'
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
  return <Modal title="Crop screenshot" onClose={onClose}>
    <div class={styles.cropper}>{url && <img ref={image} src={url} alt="Select an area of the screenshot" onError={() => setError('Could not load this screenshot.')} />}</div>
    <div class={styles.actions}>
      <Button accent onClick={crop} disabled={!ready}>Crop</Button>
      <Button onClick={onClose}>Close</Button>
    </div>
    {error && <p role="alert">{error}</p>}
  </Modal>
}
