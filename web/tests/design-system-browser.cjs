// Optional browser checks: use PLAYWRIGHT_MODULE and a running Vite server
// through CHAT_TEST_URL, as with chat-browser.cjs.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright/test')
const assert = require('node:assert/strict')
const base = process.env.CHAT_TEST_URL || 'http://127.0.0.1:5174'
const user = { username: 'alice', nickname: 'Alice', nameColor: '#f90', avatarUrl: '', admin: true, verified: true, disabled: false, createdAt: 1000 }
const permission = { room: 'default', username: 'bob', remote: true, image: true, upload: false, trusted: false, invited: true, banned: false, inviteName: 'Friends', bannedUntil: null }

;(async () => {
  const browser = await chromium.launch({ headless: true })
  const errors = []
  try {
    for (const theme of ['dark', 'light']) {
      const page = await browser.newPage({ viewport: { width: 1280, height: 900 } })
      page.on('pageerror', e => errors.push(e.message))
      await page.addInitScript(theme => localStorage.setItem('preferences', JSON.stringify({ theme })), theme)
      let signedIn = false
      await page.route('**/api/**', route => {
        const path = new URL(route.request().url()).pathname
        const data = path === '/api/me' ? { user: signedIn ? user : null } : path === '/api/settings' ? { message: '', registration: 'open' } : path === '/api/admin/users' ? [user] : path === '/api/admin/permissions' ? [permission] : path === '/api/rooms' ? [{ name: 'default', open: true, access: 'public', userCount: 1 }] : {}
        return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })
      })
      const focus = async (control, visible = control) => {
        await page.keyboard.press('Tab')
        await control.focus()
        assert.equal(await control.evaluate(el => el.matches(':focus-visible')), true)
        const ring = await visible.evaluate(el => { const s = getComputedStyle(el); return [s.outlineStyle, s.outlineWidth, s.outlineOffset, s.outlineColor] })
        assert.deepEqual(ring.slice(0, 3), ['solid', '2px', '2px'])
        return ring
      }
      await page.goto(`${base}/login`)
      const input = await focus(page.getByLabel('Username', { exact: true }))
      assert.deepEqual(await focus(page.getByRole('button', { name: 'Log in', exact: true })), input)
      assert.deepEqual(await focus(page.getByRole('link', { name: 'Sign up', exact: true }).last()), input)
      assert.deepEqual(await focus(page.getByRole('button', { name: 'Settings', exact: true })), input)
      await page.getByRole('button', { name: 'Settings', exact: true }).click()
      const dialog = page.getByRole('dialog')
      const radio = dialog.locator('input[type=radio]').first()
      assert.deepEqual(await focus(radio, radio.locator('..')), input)
      assert.deepEqual(await focus(dialog.getByRole('switch').first()), input)
      assert.deepEqual(await focus(dialog.getByRole('button', { name: 'Close', exact: true })), input)
      // Roving controls keep their indicator when arrows focus a tabindex=-1 item.
      await dialog.getByRole('radio', { name: 'Orange', exact: true }).focus()
      await page.keyboard.press('ArrowRight')
      await focus(dialog.getByRole('radio', { name: 'Blurple', exact: true }))
      await dialog.getByRole('button', { name: 'Close', exact: true }).click()
      signedIn = true
      await page.goto(`${base}/profile`)
      await page.locator('input[maxlength="12"]').waitFor()
      await focus(page.getByLabel('Nickname colour as hex'), page.locator('[data-focus-ring]').filter({ has: page.getByLabel('Nickname colour as hex') }))
      const colour = page.locator('.react-colorful__interactive').first()
      await focus(colour, colour.locator('.react-colorful__pointer'))
      await page.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'Chat', exact: true }).click()
      await focus(page.getByRole('slider').first())
      await page.getByRole('button', { name: 'Close', exact: true }).click()
      await page.goto(`${base}/admin/permissions`)
      const quiet = page.getByLabel('Invite name for bob', { exact: true })
      await quiet.waitFor()
      const widths = async () => Promise.all([page.getByLabel('Username for new permission'), page.getByLabel('Invite name for new permission'), quiet].map(input => input.evaluate(el => getComputedStyle(el).width)))
      const before = await widths()
      const remove = page.getByRole('button', { name: 'Delete the permission of bob' })
      await remove.hover()
      await page.waitForTimeout(180)
      const hover = await remove.evaluate(el => { const s = getComputedStyle(el); return [s.backgroundColor, s.color] })
      // Reverse stylesheet order: consumer widths and destructive hover still win.
      await page.evaluate(() => [...document.querySelectorAll('style[data-vite-dev-id]')].reverse().forEach(style => document.head.append(style)))
      assert.deepEqual(await widths(), before)
      await remove.hover()
      await page.waitForTimeout(180)
      assert.deepEqual(await remove.evaluate(el => { const s = getComputedStyle(el); return [s.backgroundColor, s.color] }), hover)
      await focus(page.getByLabel('remote for bob'))
      await page.setViewportSize({ width: 390, height: 900 })
      await page.goto(`${base}/admin/accounts`)
      const search = page.getByLabel('Search accounts')
      await search.waitFor()
      const mobileWidth = await search.evaluate(el => el.getBoundingClientRect().width)
      await page.evaluate(() => [...document.querySelectorAll('style[data-vite-dev-id]')].reverse().forEach(style => document.head.append(style)))
      assert.equal(await search.evaluate(el => el.getBoundingClientRect().width), mobileWidth)
      await page.close()
    }
    assert.deepEqual(errors, [])
    console.log('Passed: focus on pages and windows in both themes; input widths and delete hover survive reversed stylesheet order.')
  } finally { await browser.close() }
})().catch(e => { console.error(e); process.exitCode = 1 })
