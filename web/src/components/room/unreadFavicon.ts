// A room-local favicon. Removing it restores any icon provided by the shell.
export function unreadFavicon() {
  const link = document.createElement('link')
  link.rel = 'icon'
  link.type = 'image/png'
  link.sizes.add('64x64')
  const canvas = document.createElement('canvas')
  canvas.width = canvas.height = 64
  const context = canvas.getContext('2d')
  const logo = new Image()
  let unread = 0
  const draw = () => {
    if (!unread || !context) { link.remove(); return }
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
    context.fillStyle = tokens.getPropertyValue('--color-orange').trim()
    context.beginPath()
    context.arc(45, 45, 19, 0, Math.PI * 2)
    context.fill()
    context.fillStyle = tokens.getPropertyValue('--color-on-orange').trim()
    context.font = 'bold 20px Arial'
    context.textAlign = 'center'
    context.textBaseline = 'middle'
    context.fillText(unread > 99 ? '99+' : String(unread), 45, 45)
    link.href = canvas.toDataURL('image/png')
    if (!link.isConnected) document.head.append(link)
  }
  logo.onload = draw
  return {
    setCount(count: number) {
      unread = count
      if (count && !logo.src) logo.src = '/png/favicon.png'
      draw()
    },
    dispose() { logo.onload = null; link.remove() },
  }
}
