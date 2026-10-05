import { useEffect, useRef, useState } from 'preact/hooks'
import Cropper from 'cropperjs'
import { Modal } from '../Modal'
import { Button } from '../Button'
import { Icon } from '../ui/Icon'
import { Notice } from '../ui/Notice'
import { cropKeyboard } from '../ui/cropKeyboard'
import cropStyles from '../ui/Cropper.module.css'
import styles from './AvatarChooser.module.css'

export function AvatarChooser({
  avatar,
  disabled,
  onCrop,
}: {
  avatar: string
  disabled: boolean
  onCrop: (blob: Blob) => void
}) {
  const input = useRef<HTMLInputElement>(null),
    image = useRef<HTMLImageElement>(null),
    cropper = useRef<Cropper | null>(null),
    pendingCrop = useRef<HTMLCanvasElement | null>(null)
  const [source, setSource] = useState(''),
    [error, setError] = useState(''),
    [ready, setReady] = useState(false)
  useEffect(() => {
    if (!source || !image.current) return
    setReady(false)
    cropper.current = new Cropper(image.current, {
      aspectRatio: 1,
      zoomable: false,
      autoCropArea: 0.3,
      ready: () => setReady(true),
    })
    return () => {
      pendingCrop.current = null
      cropper.current?.destroy()
      cropper.current = null
      URL.revokeObjectURL(source)
    }
  }, [source])
  function close() {
    pendingCrop.current = null
    setSource('')
  }
  function select(file?: File) {
    setError('')
    if (!file) return
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      setError('Please select a PNG, JPEG or WebP image.')
      return
    }
    pendingCrop.current = null
    setSource(URL.createObjectURL(file))
  }
  function crop() {
    try {
      const canvas = cropper.current?.getCroppedCanvas()
      if (!canvas) {
        setError('Could not crop this image.')
        return
      }
      pendingCrop.current = canvas
      canvas.toBlob((blob) => {
        if (pendingCrop.current !== canvas) return
        pendingCrop.current = null
        if (!blob) {
          setError('Could not crop this image.')
          return
        }
        onCrop(blob)
        close()
      }, 'image/png')
    } catch {
      setError('Could not crop this image. Please choose another image.')
    }
  }
  return (
    <>
      <div class={styles.chooser}>
        <button
          class={styles.avatar}
          type="button"
          disabled={disabled}
          onClick={() => input.current?.click()}
          aria-label="Change avatar"
        >
          <img src={avatar || '/png/default_avatar.png'} alt="Avatar" />
          <span class={styles.overlay}>Change</span>
          <span class={styles.badge}><Icon name="upload" size={14} /></span>
        </button>
        <Button size="sm" disabled={disabled} onClick={() => input.current?.click()}>Change avatar</Button>
      </div>
      <input
        class={styles.input}
        type="file"
        accept="image/png,image/jpeg,image/webp"
        ref={input}
        onChange={(e) => {
          select(e.currentTarget.files?.[0])
          e.currentTarget.value = ''
        }}
      />
      {source && (
        <Modal title="Crop avatar" size="lg" onClose={close}>
          <div class={`${cropStyles.frame} ${styles.cropper}`} tabIndex={0} role="group" aria-label="Crop area: arrow keys move, Shift and arrow keys resize"
            onKeyDown={(e) => cropKeyboard(e, cropper.current, true)}>
            <img
              ref={image}
              src={source}
              alt="Crop your new avatar"
              onError={() =>
                setError(
                  'Could not load this image. Please choose another image.',
                )
              }
            />
          </div>
          <p class={cropStyles.hint}>Arrow keys move the crop; Shift + arrow keys resize it.</p>
          <div class={cropStyles.actions}>
            <Button onClick={close}>Close</Button>
            <Button variant="primary" disabled={!ready} onClick={crop}>
              Crop
            </Button>
          </div>
          {error && <Notice compact tone="error">{error}</Notice>}
        </Modal>
      )}
      {!source && error && <Notice compact tone="error">{error}</Notice>}
    </>
  )
}
