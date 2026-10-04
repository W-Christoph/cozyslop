import styles from './Avatar.module.css'

export function Avatar({ src, size = 40, alt = '' }: { src?: string; size?: number; alt?: string }) {
  return <img class={styles.avatar} src={src || '/png/default_avatar.png'} alt={alt} width={size} height={size}
    style={{ width: size, height: size }} />
}
