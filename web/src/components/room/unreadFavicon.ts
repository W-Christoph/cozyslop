// The page's favicon with an unread count drawn onto it. The icon link stays
// in place and only its address changes: browsers follow that, but keep
// showing an icon whose link was removed.
const ICON = '/png/favicon.png'

export function unreadFavicon() {
  let link = document.querySelector<HTMLLinkElement>('link[rel~="icon"]')
  if (!link) {
    link = document.createElement('link')
    link.rel = 'icon'
    link.type = 'image/png'
    link.href = ICON
    document.head.append(link)
  }
  const icon = link
  const plain = icon.href
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 64
  const context = canvas.getContext('2d')
  const logo = new Image()
  let unread = 0
  const draw = () => {
    if (!unread || !context) { icon.href = plain; return }
    const tokens = getComputedStyle(document.documentElement)
    context.clearRect(0, 0, 64, 64)
    if (logo.complete && logo.naturalWidth) context.drawImage(logo, 0, 0, 64, 64)
    else {
      context.fillStyle = tokens.getPropertyValue('--color-button').trim()
      context.beginPath()
      context.arc(32, 32, 30, 0, Math.PI * 2)
      context.fill()
      context.fillStyle = tokens.getPropertyValue('--color-crop-white').trim()
      context.font = 'bold 44px Arial'
      context.textAlign = 'center'
      context.textBaseline = 'middle'
      context.fillText('C', 28, 31)
    }
    // The badge of the old CozyCast (favico.js): the lower right 60% of the
    // icon, widening to the left for two and for three digits.
    const text = unread > 999 ? `${unread > 9999 ? 9 : Math.floor(unread / 1000)}k+` : String(unread)
    const digits = String(unread).length
    const h = 64 * 0.6, y = 64 * 0.4
    const w = h * (digits === 1 ? 1 : digits === 2 ? 1.4 : 1.65), x = 64 - w
    context.fillStyle = tokens.getPropertyValue('--color-unread').trim()
    context.beginPath()
    context.arc(x + h / 2, y + h / 2, h / 2, Math.PI / 2, Math.PI * 1.5)
    context.arc(x + w - h / 2, y + h / 2, h / 2, Math.PI * 1.5, Math.PI / 2)
    context.fill()
    context.fillStyle = tokens.getPropertyValue('--color-crop-white').trim()
    context.font = `bold ${Math.floor(h * (unread > 99 ? 0.85 : 1))}px sans-serif`
    context.textAlign = 'center'
    context.textBaseline = 'alphabetic'
    context.fillText(text, Math.floor(x + w / 2), Math.floor(y + h - h * (unread > 999 ? 0.2 : 0.15)))
    icon.href = canvas.toDataURL('image/png')
  }
  logo.onload = draw
  return {
    setCount(count: number) {
      unread = count
      if (count && !logo.src) logo.src = ICON
      draw()
    },
    dispose() { logo.onload = null; icon.href = plain },
  }
}
