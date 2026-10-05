// Optional contrast probes for every accent, using the real component modules.
// Uses the same PLAYWRIGHT_MODULE, CHAT_TEST_URL and UX_OUTPUT as ux-browser.cjs.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright/test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const base = process.env.CHAT_TEST_URL || 'http://127.0.0.1:5176'
const output = process.env.UX_OUTPUT || '/tmp/cozycast-ux'

;(async () => {
  const browser = await chromium.launch({ headless: true })
  try {
    const page = await browser.newPage()
    await page.route('**/api/**', route => route.fulfill({ json: new URL(route.request().url()).pathname === '/api/me' ? { user: null } : { registration: 'open', message: '' } }))
    await page.goto(`${base}/login`)
    const rows = await page.evaluate(async () => {
      const button = (await import('/src/components/Button.module.css')).default
      const badge = (await import('/src/components/ui/Badge.module.css')).default
      const notice = (await import('/src/components/ui/Notice.module.css')).default
      const field = (await import('/src/components/ui/Field.module.css')).default
      const toggle = (await import('/src/components/ui/Switch.module.css')).default
      const probe = document.createElement('div')
      probe.dataset.ui = ''
      probe.style.cssText = 'position:fixed;inset:0;background:var(--bg-surface);z-index:9999'
      document.body.append(probe)
      const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1
      const ctx = canvas.getContext('2d')
      const rgba = color => {
        ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = color; ctx.fillRect(0, 0, 1, 1)
        const data = ctx.getImageData(0, 0, 1, 1).data
        return [data[0], data[1], data[2], data[3] / 255]
      }
      const blend = (a, b) => a.slice(0, 3).map((v, i) => v * a[3] + b[i] * (1 - a[3]))
      const luminance = color => color.map(v => v / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4).reduce((sum, v, i) => sum + v * [.2126, .7152, .0722][i], 0)
      const ratio = (a, b) => (Math.max(luminance(a), luminance(b)) + .05) / (Math.min(luminance(a), luminance(b)) + .05)
      const rows = []
      for (const theme of ['dark', 'light']) for (const accent of ['orange', 'blurple', 'blue', 'teal', 'green', 'pink', 'red']) {
        document.documentElement.dataset.theme = theme
        document.documentElement.dataset.accent = accent
        for (const [kind, styles, variants] of [
          ['button', button, ['primary', 'secondary', 'ghost', 'danger', 'dangerOutline', 'dangerGhost']],
          ['badge', badge, ['neutral', 'accent', 'success', 'danger', 'warning']],
          ['notice', notice, ['info', 'error', 'success']],
        ]) for (const variant of variants) {
          const tag = kind === 'button' ? 'button' : 'span'
          probe.innerHTML = `<${tag} class="${styles[kind]} ${styles[variant]}">Sample</${tag}>`
          const style = getComputedStyle(probe.firstElementChild)
          const background = blend(rgba(style.backgroundColor), rgba(getComputedStyle(probe).backgroundColor))
          rows.push({ theme, accent, kind, variant, ratio: +ratio(rgba(style.color).slice(0, 3), background).toFixed(2), required: 4.5 })
        }
        const style = getComputedStyle(probe)
        for (const checked of [false, true]) {
          probe.innerHTML = `<button class="${toggle.switch}" aria-checked="${checked}"><span class="${toggle.thumb}"></span></button>`
          const track = getComputedStyle(probe.firstElementChild), thumb = getComputedStyle(probe.firstElementChild.firstElementChild)
          rows.push({ theme, accent, kind: 'boundary', variant: `switch thumb ${checked ? 'on' : 'off'}`, ratio: +ratio(rgba(thumb.backgroundColor).slice(0, 3), rgba(track.backgroundColor).slice(0, 3)).toFixed(2), required: 3 })
        }
        probe.innerHTML = `<input type="checkbox" class="${field.checkbox}">`
        const checkbox = getComputedStyle(probe.firstElementChild)
        rows.push({ theme, accent, kind: 'boundary', variant: 'checkbox border', ratio: +ratio(rgba(checkbox.borderColor).slice(0, 3), rgba(checkbox.backgroundColor).slice(0, 3)).toFixed(2), required: 3 })
        for (const [label, token, surface] of [
          ['input', '--border-control', '--bg-input'], ['checkbox', '--border-control', '--bg-surface'],
          ['radio', '--border-control', '--bg-surface'], ['switch', '--border-control', '--bg-surface'],
          ['selected radio', '--accent-text', '--bg-surface'], ['focus', '--accent-text', '--bg-raised'],
        ]) rows.push({ theme, accent, kind: 'boundary', variant: label, ratio: +ratio(rgba(style.getPropertyValue(token)).slice(0, 3), rgba(style.getPropertyValue(surface)).slice(0, 3)).toFixed(2), required: 3 })
      }
      const chatInput = (await import('/src/components/chat/ChatInput.module.css')).default
      for (const theme of ['default', 'dark', 'legacy', 'light']) {
        document.documentElement.dataset.theme = theme
        probe.innerHTML = `<div class="${chatInput.wrapper}"><textarea class="${chatInput.textarea}" placeholder="Message"></textarea></div>`
        const input = probe.querySelector('textarea'), placeholder = getComputedStyle(input, '::placeholder')
        const background = rgba(getComputedStyle(input.parentElement).backgroundColor)
        rows.push({ theme, kind: 'chat placeholder', variant: 'Message', ratio: +ratio(blend(rgba(placeholder.color), background), background.slice(0, 3)).toFixed(2), required: 4.5 })
      }
      probe.remove()
      return rows
    })
    // Hover can change both colours, so measure it after the transition settles.
    for (const theme of ['dark', 'light']) for (const accent of ['orange', 'blurple', 'blue', 'teal', 'green', 'pink', 'red']) {
      for (const variant of ['primary', 'secondary', 'ghost', 'danger', 'dangerOutline', 'dangerGhost']) {
        await page.evaluate(async ({ theme, accent, variant }) => {
          document.documentElement.dataset.theme = theme; document.documentElement.dataset.accent = accent
          const styles = (await import('/src/components/Button.module.css')).default
          let probe = document.getElementById('hover-probe')
          if (!probe) { probe = document.createElement('div'); probe.id = 'hover-probe'; probe.dataset.ui = ''; document.body.append(probe) }
          probe.style.cssText = 'position:fixed;inset:0;background:var(--bg-surface);z-index:9999'
          probe.innerHTML = `<button class="${styles.button} ${styles[variant]}">Sample</button>`
        }, { theme, accent, variant })
        await page.locator('#hover-probe button').hover()
        await page.waitForTimeout(180)
        const reading = await page.locator('#hover-probe button').evaluate(el => {
          const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1
          const ctx = canvas.getContext('2d')
          const rgba = color => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = color; ctx.fillRect(0, 0, 1, 1); return [...ctx.getImageData(0, 0, 1, 1).data] }
          const style = getComputedStyle(el), fg = rgba(style.color), bg = rgba(style.backgroundColor), base = rgba(getComputedStyle(el.parentElement).backgroundColor)
          const luminance = color => color.slice(0, 3).map(v => v / 255).map(v => v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4).reduce((sum, v, i) => sum + v * [.2126, .7152, .0722][i], 0)
          const a = luminance(fg), b = luminance(bg.slice(0, 3).map((v, i) => v * bg[3] / 255 + base[i] * (1 - bg[3] / 255)))
          return +((Math.max(a, b) + .05) / (Math.min(a, b) + .05)).toFixed(2)
        })
        rows.push({ theme, accent, kind: 'button hover', variant, ratio: reading, required: 4.5 })
      }
    }
    fs.mkdirSync(output, { recursive: true })
    fs.writeFileSync(`${output}/contrast-probes.json`, JSON.stringify(rows, null, 2))
    const failures = rows.filter(row => row.ratio < row.required)
    assert.equal(failures.length, 0, JSON.stringify(failures))
    console.log(`Passed: ${rows.length} contrast probes across both themes and all seven accents.`)
  } finally { await browser.close() }
})().catch(e => { console.error(e); process.exitCode = 1 })
