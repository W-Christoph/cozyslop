// Tiny typed event emitter, so the protocol code needs no dependencies.

type Listener = (...args: any[]) => void

export class Emitter<Events extends { [K in keyof Events]: Listener }> {
  private listeners = new Map<keyof Events, Set<Listener>>()

  on<K extends keyof Events>(event: K, fn: Events[K]): () => void {
    let set = this.listeners.get(event)
    if (!set) this.listeners.set(event, (set = new Set()))
    set.add(fn)
    return () => set.delete(fn)
  }

  protected emit<K extends keyof Events>(event: K, ...args: Parameters<Events[K]>) {
    this.listeners.get(event)?.forEach((fn) => fn(...args))
  }
}
