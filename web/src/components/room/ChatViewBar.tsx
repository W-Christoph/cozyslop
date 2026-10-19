import type { ComponentChildren } from 'preact'
import { preferences } from '../../app/state'
import { StreamPreview } from './StreamPreview'
import { useRoomStore } from './RoomContext'
import styles from './ChatViewBar.module.css'

function Action({ icon, label, title, primary = false, onClick, children }: { icon: string; label: string; title?: string; primary?: boolean; onClick: () => void; children: ComponentChildren }) {
  return <button type="button" class={`${styles.action} ${primary ? styles.primary : ''}`} aria-label={label} title={title} onClick={onClick}>
    <img src={`/svg/${icon}.svg`} alt="" /><span class={styles.label}>{children}</span>
  </button>
}

// The ways on from the chat view. Paused: listen (sound only for now, the
// chat stays in front) or watch. Listening: stop, or watch. With the setting
// Sound only the stream has no picture anyway: just play and pause.
// corner: in the room page's grid, beside the toolbar under the chat
// (wide screens); otherwise in the bar.
export function ChatViewActions({ corner = false }: { corner?: boolean }) {
  const store = useRoomStore()
  const paused = store.paused.value
  const soundOnly = preferences.value.audioOnly
  const actions = store.desktopOffline.value ? null : soundOnly
    ? paused
      ? <Action icon="play_button" label="Play" primary onClick={() => store.watch()}>Play</Action>
      : <Action icon="pause_button" label="Pause" onClick={() => store.pause()}>Pause</Action>
    : <>
      {paused
        ? <Action icon="volume-up" label="Listen: sound only" title="Only the sound, the chat stays in front" onClick={() => store.listen()}>Listen</Action>
        : <Action icon="pause_button" label="Stop listening" onClick={() => store.pause()}>Stop</Action>}
      <Action icon="play_button" label="Watch the stream" primary onClick={() => store.watch()}>Watch</Action>
    </>
  // The corner keeps its background without buttons (desktop offline).
  if (corner) return <div class={styles.corner}><div class={styles.actions}>{actions}</div></div>
  return actions && <div class={styles.actions}>{actions}</div>
}

// The top of the chat view: why there is no picture and what is on the
// desktop, with the ways on (ChatViewActions) unless the room page has them
// in its corner.
// preview: the still sits in the bar (phones, which have no column for it).
export function ChatViewBar({ preview = false, actions = true }: { preview?: boolean; actions?: boolean }) {
  const store = useRoomStore()
  const offline = store.desktopOffline.value
  const paused = store.paused.value
  const soundOnly = preferences.value.audioOnly
  const title = store.windowTitle.value
  const heading = offline ? 'The room’s desktop is offline' : paused ? 'Stream paused' : soundOnly ? 'Sound only' : 'Listening'
  const detail = offline ? 'It comes back by itself. Chat still works.'
    : title || (soundOnly ? 'The picture is off in Settings › Room.' : paused ? 'No video or sound until you listen or watch.' : 'The sound plays; the picture is off.')
  return (
    <header class={styles.bar}>
      {offline ? <img class={styles.cat} src="/svg/loading-cozy.svg" alt="" />
        : <img class={styles.icon} src={paused ? '/svg/paused.svg' : '/svg/volume-up.svg'} alt="" />}
      <div class={styles.text} role="status">
        <strong class={styles.title}>{heading}</strong>
        <span class={styles.detail} title={detail}>{detail}</span>
      </div>
      {preview && <div class={styles.preview}><StreamPreview /></div>}
      {actions && <ChatViewActions />}
    </header>
  )
}
