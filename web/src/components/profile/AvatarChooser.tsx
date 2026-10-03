import { useEffect, useRef, useState } from 'preact/hooks'
import Cropper from 'cropperjs'
import { Modal } from '../Modal'
import { Button } from '../Button'
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
    cropper = useRef<Cropper | null>(null)
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
      cropper.current?.destroy()
      cropper.current = null
      URL.revokeObjectURL(source)
    }
  }, [source])
  function select(file?: File) {
    setError('')
    if (!file) return
    if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) {
      setError('Please select a PNG, JPEG or WebP image.')
      return
    }
    setSource(URL.createObjectURL(file))
  }
  function crop() {
    try {
      const canvas = cropper.current?.getCroppedCanvas()
      if (!canvas) {
        setError('Could not crop this image.')
        return
      }
      canvas.toBlob((blob) => {
        if (!blob) {
          setError('Could not crop this image.')
          return
        }
        onCrop(blob)
        setSource('')
      }, 'image/png')
    } catch {
      setError('Could not crop this image. Please choose another image.')
    }
  }
  return (
    <>
      <button
        class={styles.avatar}
        type="button"
        disabled={disabled}
        onClick={() => input.current?.click()}
        aria-label="Upload avatar"
      >
        <img src={avatar || '/png/default_avatar.png'} alt="Avatar" />
        <span class={styles.overlay}>Upload</span>
      </button>
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
        <Modal title="Crop avatar" onClose={() => setSource('')}>
          <div class={styles.cropper}>
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
          <div class={styles.actions}>
            <Button accent disabled={!ready} onClick={crop}>
              Crop
            </Button>
            <Button onClick={() => setSource('')}>Close</Button>
          </div>
          {error && <p role="alert">{error}</p>}
        </Modal>
      )}
      {!source && error && <p role="alert">{error}</p>}
    </>
  )
}
