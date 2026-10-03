import { useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import styles from './InlineMedia.module.css'

export interface Media { type: 'image' | 'video'; url: string }
export function InlineMedia({ media, onOpen, onLoad }: { media: Media; onOpen: (media: Media) => void; onLoad: () => void }) {
  const [revealed, setRevealed] = useState(false)
  return <div class={styles.media}>
    {preferences.value.manualLoadMedia && !revealed
      ? <button type="button" class={styles.placeholder} onClick={() => setRevealed(true)}>Click to load {media.type}</button>
      : <button type="button" class={styles.preview} onClick={() => onOpen(media)} aria-label={`Open ${media.type}`}>
        {media.type === 'image'
          ? <img src={media.url} alt="Chat image" onLoad={onLoad} />
          : <video src={media.url} autoplay loop muted playsInline onLoadedData={onLoad} />}
      </button>}
  </div>
}
