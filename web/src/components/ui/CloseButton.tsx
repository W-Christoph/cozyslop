import { Icon } from './Icon'
import styles from './CloseButton.module.css'

export function CloseButton({ onClick }: { onClick: () => void }) {
  return <button class={styles.close} type="button" onClick={onClick} aria-label="Close"><Icon name="x" size={20} /></button>
}
