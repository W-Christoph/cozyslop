import { useEffect, useId, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { IconButton } from './IconButton'
import { useRoomStore } from './RoomContext'
import type { RoomWindow } from '../../pages/RoomPage'
import styles from './MoreMenu.module.css'

export function MoreMenu({ userlistHidden, onToggleUsers, onPersonalSettings, onWindow }: {
  userlistHidden: boolean
  onToggleUsers: () => void
  onPersonalSettings: () => void
  onWindow: (window: RoomWindow) => void
}) {
  const store = useRoomStore()
  const [open, setOpen] = useState(false)
  const root = useRef<HTMLDivElement>(null)
  const menu = useRef<HTMLDivElement>(null)
  const id = useId()
  const close = () => {
    setOpen(false)
    root.current?.querySelector<HTMLButtonElement>('button')?.focus()
  }
  useLayoutEffect(() => {
    if (!open || !menu.current || !root.current) return
    // Below the button: on a phone the toolbar sits right under the stream,
    // and the room is underneath it.
    const available = window.innerHeight - root.current.getBoundingClientRect().bottom - 24
    menu.current.style.maxHeight = `${Math.max(1, Math.floor(available / 44)) * 44 + 8}px`
    menu.current.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
  }, [open])
  useEffect(() => {
    if (!open) return
    const outside = (event: PointerEvent) => {
      if (!root.current?.contains(event.target as Node)) close()
    }
    const phone = window.matchMedia('(max-width: 780px) and (orientation: portrait)')
    const resize = () => { if (!phone.matches) close() }
    document.addEventListener('pointerdown', outside)
    phone.addEventListener('change', resize)
    return () => {
      document.removeEventListener('pointerdown', outside)
      phone.removeEventListener('change', resize)
    }
  }, [open])
  const rows = [
    { icon: 'users', label: userlistHidden ? 'Show users' : 'Hide users', action: onToggleUsers },
    { icon: 'settings', label: 'Personal settings', action: onPersonalSettings },
    ...(store.rights.value.upload ? [{ icon: 'folder', label: 'Files of the desktop', action: () => onWindow('files') }] : []),
    ...(store.rights.value.admin ? [{ icon: 'room-settings', label: 'Room settings', action: () => onWindow('settings') }] : []),
    { icon: 'home', label: 'Home', href: '/' },
  ]
  return <div ref={root} class={styles.more} onKeyDown={(event) => {
    if (!open) {
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') { event.preventDefault(); setOpen(true) }
      return
    }
    event.stopPropagation()
    if (event.key === 'Escape') { event.preventDefault(); close() }
    else if (event.key === 'Tab') close()
    else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
      event.preventDefault()
      const items = [...menu.current!.querySelectorAll<HTMLElement>('[role="menuitem"]')]
      const index = items.indexOf(document.activeElement as HTMLElement)
      const next = event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1
        : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
      items[next]?.focus()
    }
  }}>
    <IconButton icon="more" label="More" tooltip={false} aria-haspopup="menu" aria-expanded={open} aria-controls={open ? id : undefined}
      onClick={() => open ? close() : setOpen(true)} />
    {open && <div ref={menu} id={id} class={styles.menu} role="menu" aria-label="More">
      {rows.map((row) => {
        const content = <><img src={`/svg/${row.icon}.svg`} alt="" /><span>{row.label}</span></>
        return row.href
          ? <a key={row.label} class={styles.item} role="menuitem" tabIndex={-1} href={row.href} onClick={close}>{content}</a>
          : <button key={row.label} type="button" class={styles.item} role="menuitem" tabIndex={-1}
            onClick={() => { close(); row.action?.() }}>{content}</button>
      })}
    </div>}
  </div>
}
