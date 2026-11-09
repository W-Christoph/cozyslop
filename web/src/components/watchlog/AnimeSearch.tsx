import { useEffect, useId, useRef, useState } from 'preact/hooks'
import { watchlog, type WatchAnime } from '../../api'
import { Icon } from '../ui/Icon'
import { LoadingIndicator } from '../ui/Spinner'
import styles from './AnimeSearch.module.css'

// A search box for AniList shows, with its results under it. Arrow keys move
// through them, Enter picks one and Escape closes them.
export function AnimeSearch({ onPick, placeholder = 'Search AniList…', autoFocus = false }: {
  onPick: (anime: WatchAnime) => void
  placeholder?: string
  autoFocus?: boolean
}) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<WatchAnime[]>([])
  const [open, setOpen] = useState(false)
  const [at, setAt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const box = useRef<HTMLDivElement>(null)
  const id = useId()
  const text = query.trim()
  // Asks once typing pauses; a newer search makes older answers stale.
  useEffect(() => {
    if (text.length < 2) {
      setResults([])
      setError('')
      return
    }
    let current = true
    const timer = window.setTimeout(() => {
      setBusy(true)
      watchlog.search(text)
        .then((found) => { if (current) { setResults(found); setAt(0); setError('') } })
        .catch((e) => { if (current) setError(e instanceof Error ? e.message : 'Something went wrong.') })
        .finally(() => { if (current) setBusy(false) })
    }, 300)
    return () => { current = false; window.clearTimeout(timer) }
  }, [text])
  useEffect(() => {
    if (!open) return
    const outside = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', outside)
    return () => document.removeEventListener('mousedown', outside)
  }, [open])
  const pick = (anime: WatchAnime) => {
    onPick(anime)
    setQuery('')
    setResults([])
    setOpen(false)
  }
  const shown = open && text.length >= 2 && (results.length > 0 || error || !busy)
  return (
    <div ref={box} class={styles.search}>
      <div class={styles.field}>
        <Icon name="search" size={16} class={styles.icon} />
        <input class={styles.input} type="search" value={query} placeholder={placeholder} autoFocus={autoFocus}
          aria-label={placeholder} aria-controls={id}
          aria-activedescendant={shown && results[at] ? `${id}-${at}` : undefined}
          onInput={(e) => { setQuery(e.currentTarget.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            // Keys typed here are not the room's shortcuts.
            e.stopPropagation()
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
              e.preventDefault()
              setOpen(true)
              if (results.length) setAt((at + (e.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length)
            } else if (e.key === 'Enter' && shown && results[at]) {
              e.preventDefault()
              pick(results[at])
            } else if (e.key === 'Escape' && open) {
              e.preventDefault()
              setOpen(false)
            }
          }} />
        {busy && <LoadingIndicator />}
      </div>
      {shown && (
        <ul id={id} class={styles.results} role="listbox">
          {error ? <li class={styles.message}>{error}</li>
            : results.length === 0 ? <li class={styles.message}>Nothing found.</li>
            : results.map((anime, i) => (
              <li key={anime.id} id={`${id}-${i}`} role="option" aria-selected={i === at}
                class={`${styles.result} ${i === at ? styles.current : ''}`}
                onMouseEnter={() => setAt(i)} onMouseDown={(e) => { e.preventDefault(); pick(anime) }}>
                <Cover anime={anime} size={40} />
                <span class={styles.text}>
                  <span class={styles.title}>{anime.title}</span>
                  <span class={styles.detail}>
                    {[anime.titleEnglish !== anime.title && anime.titleEnglish, anime.year, anime.episodes && `${anime.episodes} episodes`].filter(Boolean).join(' · ')}
                  </span>
                </span>
              </li>
            ))}
        </ul>
      )}
    </div>
  )
}

// A show's cover, or a blank of its size (also when it does not load).
export function Cover({ anime, size }: { anime: Pick<WatchAnime, 'coverUrl'>; size: number }) {
  return anime.coverUrl
    ? <img class={styles.cover} src={anime.coverUrl} alt="" width={size * 0.7} height={size} loading="lazy" referrerPolicy="no-referrer"
      onError={(e) => { e.currentTarget.removeAttribute('src') }} />
    : <span class={styles.cover} style={{ width: size * 0.7, height: size }} />
}
