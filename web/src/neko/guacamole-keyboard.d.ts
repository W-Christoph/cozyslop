declare class GuacamoleKeyboard {
  constructor(element?: Element)
  /** Return false to stop the browser from handling the key. */
  onkeydown: ((keysym: number) => boolean) | null
  onkeyup: ((keysym: number) => void) | null
  /** Modifier keys currently held, as Guacamole tracks them. */
  modifiers: { ctrl: boolean; alt: boolean; shift: boolean; meta: boolean; hyper: boolean }
  listenTo(element: Element | Document): void
  reset(): void
}
export default GuacamoleKeyboard
