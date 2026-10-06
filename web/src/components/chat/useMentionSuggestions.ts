import type { RefObject } from 'preact'
import { useEffect, useId, useLayoutEffect, useRef, useState } from 'preact/hooks'
import { useRoomStore } from '../room/RoomContext'
import { activeMention, completeMention, mentionCandidates } from './mentionCompletion'
import type { User } from '../../room/protocol'

export function useMentionSuggestions(input: RefObject<HTMLTextAreaElement | null>, text: string, setText: (text: string) => void) {
  const store = useRoomStore()
  const id = useId()
  const [caret, setCaret] = useState<number | null>(null)
  const [dismissed, setDismissed] = useState('')
  const [selection, setSelection] = useState({ token: '', index: 0 })
  const pendingCaret = useRef<number | null>(null)
  const token = caret === null ? null : activeMention(text, caret)
  const signature = token ? `${token.start}:${token.query}` : ''
  const candidates = token && signature !== dismissed ? mentionCandidates(store.users.value, store.selfKey.value, token.query) : []
  const maxLength = store.self.value?.anonymous ? 250 : undefined
  const users = candidates.filter((user) => !maxLength || completeMention(text, token!, user.nickname).text.length <= maxLength)
  const index = selection.token === signature ? Math.min(selection.index, Math.max(0, users.length - 1)) : 0
  const open = users.length > 0
  function syncCaret() {
    const ta = input.current
    const next = ta && document.activeElement === ta && ta.selectionStart === ta.selectionEnd ? ta.selectionStart : null
    setCaret(next)
    if (next === null || !activeMention(ta!.value, next)) {
      setDismissed('')
      setSelection({ token: '', index: 0 })
    }
  }
  useEffect(() => {
    document.addEventListener('selectionchange', syncCaret)
    return () => document.removeEventListener('selectionchange', syncCaret)
  }, [input])
  useLayoutEffect(() => {
    if (pendingCaret.current === null || !input.current) return
    input.current.focus()
    input.current.setSelectionRange(pendingCaret.current, pendingCaret.current)
    pendingCaret.current = null
  }, [text, input])
  function close() { setCaret(null); setDismissed(signature); setSelection({ token: '', index: 0 }) }
  function accept(user: User) {
    const ta = input.current
    const current = ta && ta.selectionStart === ta.selectionEnd ? activeMention(ta.value, ta.selectionStart) : null
    if (!ta || !current) return
    const completion = completeMention(ta.value, current, user.nickname)
    if (maxLength && completion.text.length > maxLength) return
    pendingCaret.current = completion.caret
    setText(completion.text)
    close()
  }
  function onKeyDown(event: KeyboardEvent): boolean {
    if (!open || event.isComposing) return false
    if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
      event.preventDefault(); event.stopPropagation()
      setSelection({ token: signature, index: (index + (event.key === 'ArrowDown' ? 1 : -1) + users.length) % users.length })
      return true
    }
    if (event.key === 'Escape' || event.key === 'Tab' || event.key === 'Enter') {
      event.preventDefault(); event.stopPropagation()
      if (event.key === 'Escape') close()
      else accept(users[index])
      return true
    }
    return false
  }
  return {
    id, input, users, index, open, accept, close, onKeyDown, syncCaret,
    inputProps: {
      'aria-autocomplete': 'list' as const,
      'aria-haspopup': 'listbox' as const,
      'aria-expanded': open,
      'aria-controls': open ? id : undefined,
      'aria-activedescendant': open ? `${id}-${index}` : undefined,
      onFocus: () => { setDismissed(''); syncCaret() },
      onClick: syncCaret,
      onSelect: syncCaret,
      onKeyUp: syncCaret,
    },
  }
}
