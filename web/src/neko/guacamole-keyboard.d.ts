declare class GuacamoleKeyboard {
  constructor(element?: Element)
  /** Return false to stop the browser from handling the key. */
  onkeydown: ((keysym: number) => boolean) | null
  onkeyup: ((keysym: number) => void) | null
  listenTo(element: Element | Document): void
  reset(): void
}
export default GuacamoleKeyboard
