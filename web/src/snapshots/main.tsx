import { render } from 'preact'
import '../styles/tokens.css'
import '../styles/base.css'
import type { Scene } from './scene'
import { watchlogScenes } from './watchlog'
import styles from './Snapshots.module.css'

// The snapshots page: every scene, or ?scene=<name> alone; ?theme=<name>
// for another theme.
const scenes: Scene[] = [...watchlogScenes]

const params = new URLSearchParams(location.search)
document.documentElement.dataset.theme = params.get('theme') ?? 'default'
document.documentElement.dataset.accent = params.get('accent') ?? 'blue'
const only = params.get('scene')

render(
  <main class={styles.page}>
    {scenes.filter((s) => !only || s.name === only).map((s) => (
      <section key={s.name}>
        <h2 class={styles.heading}>{s.name}</h2>
        <p class={styles.about}>{s.about}</p>
        {/* A window covers the page: alone, or a link to it. */}
        {s.window && !only
          ? <a href={`?scene=${s.name}`} data-window={s.name} data-width={s.width} data-height={s.window.height}>Open this window</a>
          : <div data-snapshot={s.window ? undefined : s.name} class={`${styles.scene} ${s.surface === 'room' ? styles.room : styles.surface}`} style={{ width: `${s.width}px` }}>
            {s.render()}
          </div>}
      </section>
    ))}
  </main>,
  document.getElementById('app')!,
)
