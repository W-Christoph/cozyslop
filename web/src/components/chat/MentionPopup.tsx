import { createPortal } from 'preact/compat'
import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import { Avatar } from '../ui/Avatar'
import type { useMentionSuggestions } from './useMentionSuggestions'
import styles from './MentionPopup.module.css'

export function MentionPopup({ suggestions }: { suggestions: ReturnType<typeof useMentionSuggestions> }) {
  const { id, input, users, index, open, accept } = suggestions
  const list = useRef<HTMLDivElement>(null)
  const [container, setContainer] = useState(() => document.fullscreenElement ?? document.body)
  useLayoutEffect(() => {
    const change = () => setContainer(document.fullscreenElement ?? document.body)
    document.addEventListener('fullscreenchange', change)
    return () => document.removeEventListener('fullscreenchange', change)
  }, [])
  useLayoutEffect(() => {
    if (!open) return
    const position = () => {
      const box = input.current?.parentElement?.getBoundingClientRect()
      const element = list.current
      if (!box || !element) return
      element.style.left = `${box.left}px`
      element.style.top = `${box.top - 4}px`
      element.style.width = `${box.width}px`
      element.style.maxHeight = `${Math.max(0, Math.min(320, box.top - (window.visualViewport?.offsetTop ?? 0) - 8))}px`
    }
    position()
    const observer = new ResizeObserver(position)
    if (input.current) observer.observe(input.current)
    window.addEventListener('resize', position)
    window.addEventListener('scroll', position, true)
    window.visualViewport?.addEventListener('resize', position)
    window.visualViewport?.addEventListener('scroll', position)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', position)
      window.removeEventListener('scroll', position, true)
      window.visualViewport?.removeEventListener('resize', position)
      window.visualViewport?.removeEventListener('scroll', position)
    }
  }, [open, input, container])
  useLayoutEffect(() => {
    list.current?.children[index]?.scrollIntoView({ block: 'nearest' })
  }, [index, users.map((user) => user.key).join(' '), open])
  if (!open) return null
  return createPortal(<div id={id} ref={list} role="listbox" aria-label="Mention someone" class={styles.list}>
    {users.map((user, i) => <div key={user.key} id={`${id}-${i}`} role="option" aria-selected={i === index}
      class={styles.option} onPointerDown={(event) => event.preventDefault()} onMouseDown={(event) => event.preventDefault()}
      onClick={() => accept(user)}>
      <Avatar src={user.avatarUrl || (user.anonymous ? '/png/default_avatar_on_alpha.png' : undefined)} size={24} />
      <span>{user.nickname}</span>
    </div>)}
  </div>, container)
}
