import { cloneElement, type VNode } from 'preact'
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { preferences } from '../../app/state'
import { shortcutKeys, shortcutLabel, type ShortcutAction } from './shortcuts'
import surface from '../ui/RoomTooltipSurface.module.css'
import styles from './RoomTooltip.module.css'

export function RoomTooltip({ label, shortcut, children }: { label: string; shortcut?: ShortcutAction; children: VNode<Record<string, unknown>> }) {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [visible, setVisible] = useState(false)
  const tip = useRef<HTMLSpanElement>(null)
  const timer = useRef<number | undefined>(undefined)
  const keys = shortcut && preferences.value.shortcuts ? shortcutKeys(shortcut) : []
  const hide = () => { window.clearTimeout(timer.current); setVisible(false) }
  const show = (element: HTMLElement, delay = 0) => {
    hide()
    if (!window.matchMedia('(hover: hover)').matches) return
    setAnchor(element)
    if (delay) timer.current = window.setTimeout(() => setVisible(true), delay)
    else setVisible(true)
  }
  useEffect(() => {
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') hide() }
    window.addEventListener('keydown', escape)
    return () => { window.clearTimeout(timer.current); window.removeEventListener('keydown', escape) }
  }, [])
  useLayoutEffect(() => {
    if (!anchor || !tip.current) return
    const position = () => {
      const element = tip.current
      if (!element) return
      const rect = anchor.getBoundingClientRect()
      const width = element.getBoundingClientRect().width
      const center = rect.left + rect.width / 2
      const left = Math.max(8, Math.min(center - width / 2, window.innerWidth - width - 8))
      element.style.left = `${left + width / 2}px`
      element.style.top = `${Math.max(element.offsetHeight + 8, rect.top - 6)}px`
      element.style.setProperty('--arrow-x', `${center - left}px`)
    }
    position()
    window.addEventListener('resize', position)
    window.addEventListener('scroll', hide, true)
    return () => { window.removeEventListener('resize', position); window.removeEventListener('scroll', hide, true) }
  }, [anchor, visible, label, keys.join(' ')])
  const events = {
    onMouseEnter: (event: MouseEvent) => show(event.currentTarget as HTMLElement, 400),
    onMouseLeave: hide,
    onFocus: (event: FocusEvent) => { if ((event.target as Element).matches(':focus-visible')) show(event.currentTarget as HTMLElement) },
    onBlur: hide,
    onClick: hide,
  }
  const props: Record<string, unknown> = { title: undefined }
  for (const [name, handler] of Object.entries(events)) {
    props[name] = (event: Event) => {
      handler(event as MouseEvent & FocusEvent)
      const original = children.props[name]
      if (typeof original === 'function') original(event)
    }
  }
  return <>
    {cloneElement(children, props)}
    {anchor && <span ref={tip} aria-hidden="true" class={`${surface.surface} ${surface.top} ${styles.tooltip} ${visible ? styles.visible : ''}`}>
      {label}{keys.map((key) => <kbd key={key}>{shortcutLabel(key)}</kbd>)}
    </span>}
  </>
}
