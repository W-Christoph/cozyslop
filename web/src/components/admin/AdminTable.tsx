import type { ComponentChildren } from 'preact'
import { useLayoutEffect, useRef, useState } from 'preact/hooks'
import styles from './AdminTable.module.css'

export function AdminTable({
  headings,
  children,
}: {
  headings: string[]
  children: ComponentChildren
}) {
  const frame = useRef<HTMLDivElement>(null)
  const [scrolls, setScrolls] = useState(false)
  useLayoutEffect(() => {
    const element = frame.current
    if (!element) return
    const measure = () => setScrolls(element.scrollWidth > element.clientWidth + 1)
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    if (element.firstElementChild) observer.observe(element.firstElementChild)
    measure()
    return () => observer.disconnect()
  }, [])
  return (
    <div class={styles.frame} ref={frame}>
      <table class={`${styles.table} ${scrolls ? styles.scrolls : ''}`}>
        <thead>
          <tr>
            {headings.map((heading, i) => (
              <th key={i} scope="col">
                {heading}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
    </div>
  )
}
