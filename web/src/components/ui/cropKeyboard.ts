import type Cropper from 'cropperjs'

export function cropKeyboard(e: KeyboardEvent, cropper: Cropper | null, square = false) {
  if (!cropper || !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key)) return
  e.preventDefault()
  e.stopPropagation()
  const data = cropper.getData()
  const x = e.key === 'ArrowLeft' ? -10 : e.key === 'ArrowRight' ? 10 : 0
  const y = e.key === 'ArrowUp' ? -10 : e.key === 'ArrowDown' ? 10 : 0
  if (e.shiftKey) {
    const width = Math.max(1, data.width + (square ? x || y : x))
    const height = square ? width : Math.max(1, data.height + y)
    cropper.setData({ width, height })
  } else cropper.setData({ x: data.x + x, y: data.y + y })
}
