// Optional browser test: PLAYWRIGHT_MODULE points to an existing installation.
const { chromium, expect } = require(process.env.PLAYWRIGHT_MODULE || 'playwright/test')
const assert = require('node:assert/strict')
;(async () => {
  const browser = await chromium.launch({ headless: true })
  try {
    const page = await browser.newPage({ viewport: { width: 1000, height: 800 } })
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    await page.addInitScript(() => {
      window.subtitleWorkers = { active: 0, messages: 0 }
      const NativeWorker = window.Worker
      window.Worker = class extends NativeWorker {
        constructor(...args) { super(...args); window.subtitleWorkers.active++ }
        postMessage(...args) { window.subtitleWorkers.messages++; return super.postMessage(...args) }
        terminate() { window.subtitleWorkers.active--; return super.terminate() }
      }
    })
    await page.goto(process.env.SUBTITLE_TEST_URL || 'http://127.0.0.1:5173')
    await page.evaluate(async () => {
      // Resolve the same Preact instance Vite uses for the room context.
      const source = await (await fetch('/src/components/room/RoomContext.ts')).text()
      const preactPath = source.match(/from "([^"]*preact[^"/]*\.js[^\"]*)"/)[1]
      const { h, render } = await import(preactPath)
      const { RoomContext } = await import('/src/components/room/RoomContext.ts')
      const { SubtitleCover, coverShown } = await import('/src/components/room/SubtitleCover.tsx')
      const { preferences, updatePreferences } = await import('/src/app/state.ts')
      const canvas = document.createElement('canvas')
      canvas.width = 1280; canvas.height = 720
      const ctx = canvas.getContext('2d')
      window.drawSubtitle = (text = '') => {
        ctx.fillStyle = '#385060'; ctx.fillRect(0, 0, 1280, 720)
        ctx.font = '32px Arial'; ctx.textAlign = 'center'; ctx.lineWidth = 5
        ctx.strokeStyle = '#000'; ctx.fillStyle = '#fff'
        ctx.strokeText(text, 640, 620); ctx.fillText(text, 640, 620)
      }
      window.drawSubtitle()
      const stream = canvas.captureStream(30)
      const video = document.createElement('video')
      video.muted = true; video.autoplay = true; video.srcObject = stream
      video.style.cssText = 'width:800px;height:450px;display:block'
      document.body.innerHTML = ''
      const host = document.createElement('div')
      host.style.cssText = 'position:relative;width:800px;height:450px;--screen-ratio:16 / 9'
      document.body.append(host); host.append(video)
      const overlay = document.createElement('div'); host.append(overlay)
      const store = { stream: { value: stream }, paused: { value: false }, audioOnly: { value: false } }
      updatePreferences({ subtitleCover: true, subtitleCoverTwoBoxes: false, subtitleCoverDetection: 'region', subtitleCoverFeather: 20 })
      coverShown.value = true
      window.subtitlePrefs = { preferences, updatePreferences, coverShown }
      window.renderCover = () => render(h(RoomContext.Provider, { value: store }, h(SubtitleCover, { video: { current: video } })), overlay)
      window.renderCover()
      await video.play()
    })
    const clip = page.locator('[class*="_clip_"]')
    await expect(clip).toHaveCount(0)
    await page.evaluate(() => window.drawSubtitle('This is a subtitle for testing.'))
    await expect(clip).toHaveCount(1)
    const regionalWidth = (await clip.boundingBox()).width
    assert.ok(regionalWidth < 600, `Expected tight region, got ${regionalWidth}`)
    for (const reply of ['OK', 'No!', 'Yes']) {
      await page.evaluate(text => window.drawSubtitle(text), reply)
      await expect.poll(async () => (await clip.boundingBox())?.width).toBeLessThan(150)
      // Wait beyond disappearance hysteresis so a stale long-line match cannot pass.
      await page.waitForTimeout(700)
      await expect(clip).toHaveCount(1)
    }
    await page.evaluate(() => window.subtitlePrefs.updatePreferences({ subtitleCoverDetection: 'box' }))
    await expect.poll(async () => (await clip.boundingBox())?.width).toBe(680)
    // Feather extends 20px beyond each side of the draggable 640px box.
    const cover = page.locator('[title^="Subtitle cover:"]').first()
    assert.equal((await cover.boundingBox()).width, 640)
    assert.equal((await clip.boundingBox()).x, (await cover.boundingBox()).x - 20)
    await page.evaluate(() => window.drawSubtitle())
    await expect(clip).toHaveCount(0, { timeout: 5000 })
    await page.evaluate(() => window.subtitlePrefs.updatePreferences({ subtitleCoverDetection: 'manual', subtitleCoverTwoBoxes: true }))
    await expect(clip).toHaveCount(2)
    await page.evaluate(() => { window.subtitlePrefs.coverShown.value = false })
    await expect(clip).toHaveCount(0)
    await expect.poll(() => page.evaluate(() => window.subtitleWorkers.active)).toBe(0)
    // A missing worker must visibly fall back to manual covers.
    await page.evaluate(() => {
      window.Worker = class { constructor() { throw new Error('Worker unavailable') } }
      window.subtitlePrefs.updatePreferences({ subtitleCoverDetection: 'region' })
      window.subtitlePrefs.coverShown.value = true
    })
    await expect(clip).toHaveCount(2)
    assert.deepEqual(errors, [])
    console.log('PASS: real canvas text, regional/full-box detection, disappearance, outward feather, two boxes, toolbar off')
    // Synthetic analysis benchmark; excludes frame readback and GPU blur.
    const timing = await page.evaluate(async () => {
      const { detectSubtitle } = await import('/src/components/room/subtitleDetection.ts')
      const canvas = document.createElement('canvas'); canvas.width = 640; canvas.height = 240
      const ctx = canvas.getContext('2d'); ctx.fillStyle = '#234'; ctx.fillRect(0, 0, 640, 240)
      ctx.font = '22px Arial'; ctx.strokeStyle = '#000'; ctx.lineWidth = 3; ctx.fillStyle = '#fff'
      ctx.strokeText('This is a subtitle for testing.', 120, 160); ctx.fillText('This is a subtitle for testing.', 120, 160)
      const data = ctx.getImageData(0, 0, 640, 240).data
      for (let n = 0; n < 10; n++) detectSubtitle(data, 640, 240)
      const start = performance.now()
      for (let n = 0; n < 100; n++) detectSubtitle(data, 640, 240)
      return (performance.now() - start) / 100
    })
    console.log(`Synthetic 640×240 analysis: ${timing.toFixed(2)} ms/crop`)
  } finally { await browser.close() }
})().catch(e => { console.error(e); process.exitCode = 1 })
