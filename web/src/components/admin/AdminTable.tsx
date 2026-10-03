import type { ComponentChildren } from 'preact'
import styles from './AdminTable.module.css'

export function AdminTable({
  headings,
  children,
}: {
  headings: string[]
  children: ComponentChildren
}) {
  return (
    <div class={styles.background}>
      <table class={styles.table}>
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
