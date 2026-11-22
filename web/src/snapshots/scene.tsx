import type { ComponentChildren, JSX } from 'preact'
import { signal } from '@preact/signals'
import type { RoomStore } from '../room/store'
import styles from './Snapshots.module.css'

// A part of the UI drawn from made-up data, to look at without the state
// it needs for real (snapshots.html; npm run snapshots makes a picture of
// each).
export interface Scene {
  name: string // the picture's file name: lower case, with dashes
  about: string
  width: number // in CSS pixels
  surface: 'room' | 'page' // drawn on the room's background (chat) or a page's (windows)
  // A window (Modal) over the page: drawn alone (?scene=<name>), and its
  // picture is the whole screen, this tall.
  window?: { height: number }
  render: () => JSX.Element
}

// Several states of one part side by side, each with a label.
export function Variants({ columns, items }: { columns: number; items: [string, ComponentChildren][] }) {
  return (
    <div class={styles.variants} style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
      {items.map(([label, item]) => (
        <div key={label}>
          <div class={styles.label}>{label}</div>
          {item}
        </div>
      ))}
    </div>
  )
}

// A RoomStore for room components: the signals given, and anything else a
// function that does nothing.
export function fakeRoom(signals: Record<string, unknown>): RoomStore {
  const fields = Object.fromEntries(Object.entries(signals).map(([key, value]) => [key, signal(value)]))
  return new Proxy(fields, { get: (target, key: string) => target[key] ?? (() => {}) }) as unknown as RoomStore
}
