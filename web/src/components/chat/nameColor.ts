type Color = readonly number[]

function rgb(color: string): number[] | null {
  if (/^#[\da-f]{3}$/i.test(color)) return [...color.slice(1)].map((c) => parseInt(c + c, 16))
  if (/^#[\da-f]{6}$/i.test(color)) return [1, 3, 5].map((at) => parseInt(color.slice(at, at + 2), 16))
  const match = color.match(/^rgba?\((\d+),\s*(\d+),\s*(\d+)/)
  return match ? match.slice(1).map(Number) : null
}

function luminance(color: Color) {
  return color.reduce((sum, channel, i) => {
    const c = channel / 255
    return sum + (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4) * [0.2126, 0.7152, 0.0722][i]
  }, 0)
}

// Preserve colours that read already; mix only as far as needed for AA.
export function readableNameColor(color: string, background: string, alternate = background) {
  const foreground = rgb(color), surface = rgb(background)
  if (!foreground || !surface) return color
  const backgrounds = [surface, rgb(alternate) ?? surface].map(luminance)
  const contrast = (c: Color) => Math.min(...backgrounds.map((bg) => (Math.max(luminance(c), bg) + 0.05) / (Math.min(luminance(c), bg) + 0.05)))
  const bg = luminance(surface)
  if (contrast(foreground) >= 4.6) return color
  const end = bg > 0.179 ? 0 : 255
  const mix = (amount: number) => foreground.map((c) => Math.round(c + (end - c) * amount))
  let low = 0, high = 1
  for (let i = 0; i < 12; i++) {
    const amount = (low + high) / 2
    if (contrast(mix(amount)) >= 4.6) high = amount
    else low = amount
  }
  return `rgb(${mix(high).join(', ')})`
}

export function applyNameColors(container: HTMLElement | null) {
  if (!container) return
  const names = container.querySelectorAll<HTMLElement>('[data-chat-name]')
  // Over the stream chat is light on dark: only names too dark for that change.
  if (container.closest('[data-chat-overlay]')) {
    for (const name of names) name.style.setProperty('--readable-name-colour', readableNameColor(name.style.getPropertyValue('--name-colour'), 'rgb(0, 0, 0)'))
    return
  }
  const style = getComputedStyle(container)
  const background = style.getPropertyValue('--color-menu').trim()
  const base = rgb(background)
  if (!base) return
  const blend = (token: string, under: number[]) => {
    const color = rgb(token)
    const alpha = Number(token.match(/,\s*([\d.]+)\)$/)?.[1] ?? 1)
    return color ? color.map((c, i) => Math.round(c * alpha + under[i] * (1 - alpha))) : under
  }
  const bubble = blend(style.getPropertyValue('--color-message-bg').trim(), base)
  const hover = style.getPropertyValue('--color-message-hover').trim()
  const worst = luminance(base) > 0.179 ? blend(hover, bubble) : blend(hover, base)
  for (const name of names) {
    name.style.setProperty('--readable-name-colour', readableNameColor(name.style.getPropertyValue('--name-colour'), background, `rgb(${worst.join(', ')})`))
  }
}
