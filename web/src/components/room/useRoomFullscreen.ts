import type { RefObject } from 'preact'
import { useCallback, useEffect, useRef, useState } from 'preact/hooks'

export function useRoomFullscreen(page: RefObject<HTMLDivElement | null>) {
  const [fullscreen, setFullscreen] = useState(false)
  const [idle, setIdle] = useState(false)
  const [error, setError] = useState('')
  const timer = useRef<number | undefined>(undefined)
  const wake = useCallback(() => {
    setIdle(false)
    window.clearTimeout(timer.current)
    if (!!page.current && document.fullscreenElement === page.current) {
      timer.current = window.setTimeout(() => setIdle(true), 2400)
    }
  }, [page])
  useEffect(() => {
    const change = () => {
      setFullscreen(!!page.current && document.fullscreenElement === page.current)
      wake()
    }
    document.addEventListener('fullscreenchange', change)
    change()
    return () => {
      document.removeEventListener('fullscreenchange', change)
      window.clearTimeout(timer.current)
    }
  }, [page, wake])
  useEffect(() => {
    if (!error) return
    const timeout = window.setTimeout(() => setError(''), 5000)
    return () => window.clearTimeout(timeout)
  }, [error])
  const toggle = async () => {
    setError('')
    try {
      if (document.fullscreenElement) await document.exitFullscreen()
      else if (page.current?.requestFullscreen) await page.current.requestFullscreen()
      else setError('Fullscreen is not available in this browser.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Unable to enter fullscreen.')
    }
  }
  return { fullscreen, idle, wake, toggle, error }
}
