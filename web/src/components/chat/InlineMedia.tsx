import { useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import styles from './InlineMedia.module.css'

export interface Media { type: 'image' | 'video'; url: string }
export function InlineMedia({ media, onOpen, onLoad }: { media: Media; onOpen: (media: Media) => void; onLoad: () => void }) {
  const [revealed, setRevealed] = useState(false)
  // Width over height, once loaded: a tall picture narrows instead of being
  // squeezed into the height limit.
  const [ratio, setRatio] = useState(0)
  const limit = preferences.value.chatMediaHeight
  const size = limit > 0 ? { maxHeight: `${limit}px`, ...(ratio ? { width: `min(100%, ${Math.round(limit * ratio)}px)` } : {}) } : undefined
  return <div class={styles.media}>
    {preferences.value.manualLoadMedia && !revealed
      ? <button type="button" class={styles.placeholder} onClick={() => setRevealed(true)}>Click to load {media.type}</button>
      : <button type="button" class={styles.preview} onClick={() => onOpen(media)} aria-label={`Open ${media.type}`}>
        {media.type === 'image'
          ? <img src={media.url} alt="Chat image" style={size}
            onLoad={(e) => { const { naturalWidth: w, naturalHeight: h } = e.currentTarget; if (h) setRatio(w / h); onLoad() }} />
          : <video src={media.url} autoplay loop muted playsInline style={size}
            onLoadedData={(e) => { const { videoWidth: w, videoHeight: h } = e.currentTarget; if (h) setRatio(w / h); onLoad() }} />}
      </button>}
  </div>
}
