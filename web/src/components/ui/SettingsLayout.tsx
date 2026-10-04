import type { ComponentChildren } from 'preact'
import { useId } from 'preact/hooks'
import { Dialog } from './Dialog'
import { Icon, type IconName } from './Icon'
import styles from './SettingsLayout.module.css'

export interface NavItem {
  id: string
  label: string
  icon: IconName
  href?: string // a link instead of onSelect
  danger?: boolean
}
export interface NavGroup {
  label?: string
  items: NavItem[]
}

interface Props {
  nav: NavGroup[]
  current: string
  onSelect?: (id: string) => void
  navHeader?: ComponentChildren // above the navigation
  navFooter?: ComponentChildren // below it
  wide?: boolean // tables: let the content use the whole width
  left?: boolean // no centring: for windows where some sections are wide
  children: ComponentChildren
}

// Navigation on the left, the chosen section on the right: the frame of the
// settings windows and of the admin area.
export function SettingsLayout({ nav, current, onSelect, navHeader, navFooter, wide, left, children, titleId, onClose, page }: Props & {
  titleId?: string
  onClose?: () => void
  page?: boolean
}) {
  const title = nav.flatMap((group) => group.items).find((item) => item.id === current)?.label ?? ''
  return (
    <div class={`${styles.layout} ${page ? styles.page : ''} ${left ? styles.left : ''}`} data-page={page || undefined}>
      <nav class={styles.nav} aria-label="Sections">
        {navHeader && <div class={styles.navHeader}>{navHeader}</div>}
        <div class={styles.groups}>
          {nav.map((group, index) => (
            <div class={styles.group} key={group.label ?? index}>
              {group.label && <div class={styles.groupLabel}>{group.label}</div>}
              {group.items.map((item) => {
                const className = `${styles.item} ${item.id === current ? styles.current : ''} ${item.danger ? styles.danger : ''}`
                const content = <><Icon name={item.icon} /><span>{item.label}</span></>
                return item.href
                  ? <a key={item.id} class={className} href={item.href} aria-current={item.id === current ? 'page' : undefined}>{content}</a>
                  : <button key={item.id} type="button" class={className} aria-current={item.id === current ? 'true' : undefined}
                    onClick={() => onSelect?.(item.id)}>{content}</button>
              })}
            </div>
          ))}
        </div>
        {navFooter && <div class={styles.navFooter}>{navFooter}</div>}
      </nav>
      <div class={styles.main}>
        <header class={styles.header}>
          <h2 id={titleId} class={styles.title}>{title}</h2>
          {onClose && <button class={styles.close} type="button" onClick={onClose} aria-label="Close"><Icon name="x" size={20} /></button>}
        </header>
        <div class={styles.scroll}>
          <div class={`${styles.content} ${wide ? styles.wide : ''}`}>{children}</div>
        </div>
      </div>
    </div>
  )
}

export function SettingsWindow({ onClose, ...props }: Props & { onClose: () => void }) {
  const id = useId()
  return (
    <Dialog id={id} labelledBy={id} onClose={onClose} class={styles.window}>
      <SettingsLayout {...props} titleId={id} onClose={onClose} />
    </Dialog>
  )
}
