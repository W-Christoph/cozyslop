// Optional UX audit: PLAYWRIGHT_MODULE, CHAT_TEST_URL and UX_OUTPUT point to
// an existing Playwright installation, Vite and the evidence directory.
const { chromium, expect } = require(process.env.PLAYWRIGHT_MODULE || 'playwright/test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const base = process.env.CHAT_TEST_URL || 'http://127.0.0.1:5176'
const output = process.env.UX_OUTPUT || '/tmp/cozycast-ux'
const self = { key: 'u:1', username: 'alice', nickname: 'Alice', nameColor: '#f90', avatarUrl: '', anonymous: false, admin: true, active: true, muted: false, joinedAt: 1, lastSeen: 1000, verified: true }
const bob = { ...self, key: 'u:2', username: 'bob', nickname: 'Bob', nameColor: '#4aa', admin: false }
const settings = { name: 'Cozy movie night', access: 'public', hidden: false, remoteOwnership: false, defaultRemote: true, defaultImage: true, defaultUpload: true, screen: '', quality: 'medium' }
const permission = { room: 'default', username: 'bob', remote: true, image: true, upload: false, trusted: false, invited: true, banned: true, inviteName: 'Movie friends', bannedUntil: 1791320400 }
const invite = { code: 'movie-night', room: 'default', temporary: false, name: 'Movie friends', remote: true, image: true, upload: false, uses: 2, maxUses: 10, expiresAt: null, createdAt: 1791200000, valid: true, path: '/invite/movie-night' }
const msg = (id, author, body) => ({ id, author: author.key, nickname: author.nickname, nameColor: author.nameColor, anonymous: false, type: 'text', body, edited: false, time: Date.now() })
const results = [], keyboard = [], contrast = [], errors = []

async function setup(browser, theme, width) {
  const page = await browser.newPage({ viewport: { width, height: 900 }, hasTouch: width === 390 })
  let signedIn = true, room
  page.on('pageerror', e => errors.push(e.message))
  page.on('dialog', d => { errors.push(`Native ${d.type()} dialog`); d.dismiss() })
  await page.addInitScript(({ theme }) => {
    localStorage.setItem('preferences', JSON.stringify({ theme, manualLoadMedia: false }))
    window.RTCPeerConnection = class {
      connectionState = 'new'
      async setRemoteDescription() {}
      async createAnswer() { return { type: 'answer', sdp: 'test' } }
      async addIceCandidate() {}
      async setLocalDescription() {
        this.connectionState = 'connected'; this.onconnectionstatechange?.()
        this.ondatachannel?.({ channel: { readyState: 'open', send() {} } })
        const canvas = document.createElement('canvas'); canvas.width = 640; canvas.height = 360
        const draw = () => { const ctx = canvas.getContext('2d'); ctx.fillStyle = '#235'; ctx.fillRect(0, 0, 640, 360); ctx.fillStyle = 'white'; ctx.font = '32px Arial'; ctx.fillText('Cozy movie night', 180, 180) }
        draw(); this.ontrack?.({ streams: [canvas.captureStream(5)] }); this.timer = setInterval(draw, 200)
      }
      close() { clearInterval(this.timer) }
    }
  }, { theme })
  await page.route('**/api/**', route => {
    const path = new URL(route.request().url()).pathname
    const data = path === '/api/me' ? { user: signedIn ? self : null }
      : path === '/api/settings' ? { message: 'Welcome! Be kind and enjoy the movie.', registration: 'open', sourceUrl: '' }
      : path === '/api/rooms' ? [{ name: 'default', open: true, access: 'public', userCount: 2 }, { name: 'Cinema', open: false, access: 'invite', userCount: 0 }]
      : path === '/api/admin/users' ? [{ ...self, disabled: false, createdAt: 1791200000 }, { ...bob, disabled: true, createdAt: 1791200000 }]
      : path === '/api/admin/permissions' ? [permission, { ...permission, username: 'charlie', banned: false, invited: false }]
      : path === '/api/admin/invites' ? [invite, { ...invite, code: 'expired', name: 'Expired invitation', valid: false, expiresAt: 1791200000 }]
      : path === '/api/admin/bans' ? [{ id: 1, room: 'default', ip: '192.0.2.123', bannedUntil: null }]
      : path.endsWith('/stream-options') ? { screens: ['1280x720@30', '1920x1080@30'], streams: ['b2500-s100-veryfast', 'b2500-s75-veryfast', 'b5000-s100-medium'] } : path.endsWith('/grants') ? [] : path.endsWith('/settings') ? settings : {}
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(data) })
  })
  await page.routeWebSocket('**/api/rooms/**', ws => {
    room = ws
    setTimeout(() => {
      const colors = ['#f90', '#4aa', '#fff', '#000', '#ff0', '#00f', '#f00', '#080']
      ws.send(JSON.stringify({ type: 'welcome', clientId: 'tab', self, rights: { admin: true, remote: true, image: true, upload: true, trusted: true }, settings, users: [self, bob], history: colors.map((color, i) => msg(i + 1, { ...bob, key: `u:${i + 2}`, nickname: `Colour ${i + 1}`, nameColor: color }, `A readable message in colour ${color}.`)), remote: null, restart: true }))
      ws.send(JSON.stringify({ type: 'neko', token: 'test', path: '/neko/default' }))
    }, 20)
  })
  await page.routeWebSocket('**/neko/**', ws => ws.onMessage(data => {
    const event = JSON.parse(data).event
    if (event === 'signal/request') {
      // neko's heartbeat: the client drops a connection silent for 25s.
      const beat = setInterval(() => ws.send(JSON.stringify({ event: 'system/heartbeat' })), 10_000)
      ws.onClose(() => clearInterval(beat))
      ws.send(JSON.stringify({ event: 'system/init', payload: { session_id: 'tab', sessions: { tab: { profile: { can_host: true } } }, screen_size: { width: 640, height: 360, rate: 30 }, control_host: { has_host: false } } }))
      ws.send(JSON.stringify({ event: 'signal/provide', payload: { sdp: 'test' } }))
    }
    if (event === 'filetransfer/update') ws.send(JSON.stringify({ event: 'filetransfer/update', payload: { files: [{ name: 'Movie night notes.txt', type: 'file', size: 2048 }, { name: 'A very long movie filename for the mobile download window.mp4', type: 'file', size: 1048576 }] } }))
  }))
  return { page, signIn: value => { signedIn = value }, send: data => room.send(JSON.stringify(data)) }
}

// Resolve CSS colour functions in the browser, then composite alpha and each
// ancestor's opacity before calculating WCAG relative luminance.
async function measure(page, screen, theme, width) {
  const rows = await page.evaluate(() => {
    const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1
    const ctx = canvas.getContext('2d', { willReadFrequently: true })
    const rgba = color => { ctx.clearRect(0, 0, 1, 1); ctx.fillStyle = color; ctx.fillRect(0, 0, 1, 1); const c = [...ctx.getImageData(0, 0, 1, 1).data]; return [c[0], c[1], c[2], c[3] / 255] }
    const over = (a, b) => { const alpha = a[3] + b[3] * (1 - a[3]); return [0, 1, 2].map(i => alpha ? (a[i] * a[3] + b[i] * b[3] * (1 - a[3])) / alpha : 0).concat(alpha) }
    const luminance = c => c.slice(0, 3).map(v => { v /= 255; return v <= .04045 ? v / 12.92 : ((v + .055) / 1.055) ** 2.4 }).reduce((sum, v, i) => sum + v * [.2126, .7152, .0722][i], 0)
    const dialogs = [...document.querySelectorAll('[role=dialog]')], scope = dialogs.at(-1) || document.body
    const readings = []
    for (const el of scope.querySelectorAll('*')) {
      const style = getComputedStyle(el), rect = el.getBoundingClientRect()
      if (!rect.width || !rect.height || rect.bottom < 0 || rect.top > innerHeight || rect.right < 0 || rect.left > innerWidth || style.visibility !== 'visible' || el.closest('button:disabled,input:disabled,select:disabled,[aria-hidden=true]')) continue
      const text = [...el.childNodes].filter(n => n.nodeType === Node.TEXT_NODE).map(n => n.textContent.trim()).join(' ') || (el.matches('input:not([type=radio],[type=checkbox],[type=range],[type=file]),textarea') ? el.value : '')
      if (!text) continue
      let fg = rgba(style.color), bg = rgba(style.backgroundColor), opacity = Number(style.opacity)
      fg = over(fg, bg); fg[3] *= opacity; bg[3] *= opacity
      let invisible = opacity === 0, gradient = style.backgroundImage !== 'none'
      for (let parent = el.parentElement; parent; parent = parent.parentElement) {
        const s = getComputedStyle(parent), surface = rgba(s.backgroundColor), alpha = Number(s.opacity)
        fg = over(fg, surface); bg = over(bg, surface); fg[3] *= alpha; bg[3] *= alpha
        invisible ||= alpha === 0; gradient ||= s.backgroundImage !== 'none'
      }
      if (invisible || gradient) continue
      fg = over(fg, [255, 255, 255, 1]); bg = over(bg, [255, 255, 255, 1])
      const a = luminance(fg), b = luminance(bg), ratio = (Math.max(a, b) + .05) / (Math.min(a, b) + .05)
      const large = parseFloat(style.fontSize) >= 24 || (parseFloat(style.fontSize) >= 18.66 && parseInt(style.fontWeight) >= 700)
      readings.push({ text: text.slice(0, 70), class: el.className, color: style.color, foreground: fg, background: bg, ratio: +ratio.toFixed(2), required: large ? 3 : 4.5 })
    }
    return readings
  })
  contrast.push(...rows.map(row => ({ screen, theme, width, ...row })))
}

async function shot(page, screen, theme, width) {
  await page.waitForTimeout(260)
  if (screen.startsWith('personal-') || screen.startsWith('room-settings-')) {
    const layout = await page.evaluate(async () => (await import('/src/components/ui/SettingsLayout.module.css')).default)
    const window = page.locator(`.${layout.window}`)
    assert(await window.evaluate(el => el.scrollHeight <= el.clientHeight + 2), `${screen}: settings content clipped outside scrolling region`)
  }
  await page.screenshot({ path: `${output}/shots/${screen}-${theme}-${width}.png`, fullPage: true })
  await measure(page, screen, theme, width)
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)
  results.push({ screen, theme, width, result: overflow ? 'page overflow' : 'captured' })
  for (const scroll of await page.getByRole('dialog').last().locator('*').all()) {
    const state = await scroll.evaluate(el => { const style = getComputedStyle(el); return { overflow: /auto|scroll/.test(style.overflowY) && el.scrollHeight > el.clientHeight + 2, top: el.scrollTop } })
    if (!state.overflow) continue
    await scroll.evaluate(el => { el.scrollTop = el.scrollHeight })
    await page.screenshot({ path: `${output}/shots/${screen}-bottom-${theme}-${width}.png`, fullPage: true })
    await measure(page, `${screen}-bottom`, theme, width)
    results.push({ screen: `${screen}-bottom`, theme, width, result: 'captured' })
    await scroll.evaluate((el, top) => { el.scrollTop = top }, state.top)
  }
}

// Actual Tab traversal, including native radio groups and roving controls.
async function trap(page, name, opener, count = 1) {
  const dialog = page.getByRole('dialog').last()
  await expect(page.getByRole('dialog')).toHaveCount(count)
  assert(await dialog.evaluate(el => el.contains(document.activeElement)), `${name}: opening focus`)
  const visited = new Set(), order = []
  let start
  for (let i = 0; i < 100; i++) {
    await page.keyboard.press('Tab')
    const state = await dialog.evaluate(el => {
      const active = document.activeElement
      if (!el.contains(active)) return { inside: false }
      const target = active.closest('[data-focus-ring]') || active.querySelector('.react-colorful__pointer') || active
      const style = getComputedStyle(target)
      return { inside: true, ring: style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) >= 2, key: [...el.querySelectorAll('*')].indexOf(active), label: active.getAttribute('aria-label') || active.textContent.trim().slice(0, 40) }
    })
    assert(state.inside, `${name}: Tab escaped`)
    assert(state.ring, `${name}: no visible focus on ${state.label}`)
    if (state.key === start && visited.size > 1 || state.key === start && count === 1 && name.startsWith('media')) break
    start ??= state.key
    if (!visited.has(state.key)) order.push(state.label)
    visited.add(state.key)
  }
  for (let i = 0; i <= visited.size; i++) {
    await page.keyboard.press('Shift+Tab')
    assert(await dialog.evaluate(el => el.contains(document.activeElement)), `${name}: Shift+Tab escaped`)
  }
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(count - 1)
  if (opener) await expect(opener).toBeFocused()
  keyboard.push({ name, controls: visited.size, order, result: 'pass' })
}

async function run() {
  fs.mkdirSync(`${output}/shots`, { recursive: true })
  const browser = await chromium.launch({ headless: true })
  try {
    for (const theme of ['dark', 'light']) for (const width of [1280, 390]) {
      const h = await setup(browser, theme, width), p = h.page
      for (const [screen, path] of [['rooms', '/'], ['404', '/missing-page'], ['admin-accounts', '/admin/accounts'], ['admin-permissions', '/admin/permissions'], ['admin-invites', '/admin/invites'], ['admin-settings', '/admin/settings']]) {
        await p.goto(`${base}${path}`)
        await p.waitForTimeout(250)
        await shot(p, screen, theme, width)
        if (screen === 'admin-accounts') {
          const reset = p.getByRole('button', { name: 'Reset password of bob', exact: true })
          await reset.click(); await shot(p, 'admin-reset-password', theme, width)
          await trap(p, `reset password ${theme} ${width}`, reset)
          const remove = p.getByRole('button', { name: 'Delete bob', exact: true })
          await remove.click(); await shot(p, 'admin-delete-account', theme, width)
          await trap(p, `delete account ${theme} ${width}`, remove)
        }
        if (screen === 'admin-permissions') {
          await p.getByLabel('Ban date for bob', { exact: true }).click()
          await shot(p, 'admin-ban-date', theme, width)
          await p.getByLabel('Banned until for bob (empty means forever)').press('Escape')
        }
      }
      const menuOpener = p.locator('button[aria-haspopup=menu]')
      await menuOpener.click(); await shot(p, 'header-menu', theme, width)
      for (let i = 0; i < 10; i++) {
        await p.keyboard.press(i < 5 ? 'Tab' : 'Shift+Tab')
        assert(await p.getByRole('menu').evaluate(el => el.contains(document.activeElement)), 'header menu contains Tab')
        assert(await p.locator(':focus').evaluate(el => getComputedStyle(el).outlineStyle === 'solid'), 'header menu focus ring')
      }
      await p.keyboard.press('Escape'); await expect(menuOpener).toBeFocused()
      await menuOpener.click()
      await p.getByRole('menuitem', { name: 'My account', exact: true }).click()
      await trap(p, `header account ${theme} ${width}`, menuOpener)
      h.signIn(false); await p.goto(`${base}/login`); await shot(p, 'login', theme, width)
      h.signIn(true); await p.goto(`${base}/room/default`)
      await p.getByLabel('Chat message', { exact: true }).waitFor()
      await expect.poll(() => p.locator('video').first().evaluate(el => el.readyState)).toBeGreaterThanOrEqual(2)
      for (const chatStyle of ['classic', 'modern', 'compact']) {
        await p.evaluate(async chatStyle => (await import('/src/app/state.ts')).updatePreferences({ chatStyle }), chatStyle)
        await shot(p, `room-${chatStyle}`, theme, width)
      }
      await p.getByRole('button', { name: 'Fullscreen', exact: true }).click()
      for (const chatStyle of ['classic', 'modern', 'compact']) {
        await p.evaluate(async chatStyle => (await import('/src/app/state.ts')).updatePreferences({ chatStyle }), chatStyle)
        await shot(p, `room-overlay-${chatStyle}`, theme, width)
      }
      await p.getByRole('button', { name: 'Exit fullscreen', exact: true }).click()
      const personal = p.getByRole('button', { name: width <= 780 ? 'More' : 'Personal settings', exact: true })
      const openPersonal = async () => {
        await personal.click()
        if (width <= 780) await p.getByRole('menuitem', { name: 'Personal settings', exact: true }).click()
      }
      await openPersonal()
      for (const section of ['My account', 'Appearance', 'Chat', 'Room', 'Notifications']) {
        await p.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: section, exact: true }).click()
        await shot(p, `personal-${section.toLowerCase().replaceAll(' ', '-')}`, theme, width)
        if (section === 'My account') {
          await p.keyboard.press('Tab')
          await p.getByLabel('Nickname colour as hex').focus()
          await shot(p, 'personal-name-colour-focus', theme, width)
          await p.locator('.react-colorful__interactive').first().focus()
          await shot(p, 'personal-colour-picker-focus', theme, width)
        }
        await trap(p, `personal ${section} ${theme} ${width}`, personal)
        if (section !== 'Notifications') await openPersonal()
      }
      const roomSettings = p.getByRole('button', { name: width <= 780 ? 'More' : 'Room settings', exact: true })
      const openRoomSettings = async () => {
        await roomSettings.click()
        if (width <= 780) await p.getByRole('menuitem', { name: 'Room settings', exact: true }).click()
      }
      await openRoomSettings()
      for (const section of ['Access', 'Stream', 'Tools', 'In the room', 'Permissions', 'Invites', 'Anonymous bans']) {
        await p.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: section, exact: true }).click()
        await shot(p, `room-settings-${section.toLowerCase().replaceAll(' ', '-')}`, theme, width)
        if (section === 'Tools') {
          const restart = p.getByRole('button', { name: 'Restart', exact: true })
          await restart.click(); await shot(p, 'restart-dialog', theme, width)
          await trap(p, `restart ${theme} ${width}`, restart, 2)
        }
        if (section === 'Invites') {
          const create = p.getByRole('button', { name: 'Create invite', exact: true })
          await create.click(); await shot(p, 'invite-dialog', theme, width)
          await trap(p, `invite ${theme} ${width}`, create, 2)
        }
        if (section === 'In the room') {
          const ban = p.getByRole('button', { name: 'Ban or kick Bob', exact: true })
          await ban.click(); await shot(p, 'ban-dialog', theme, width)
          await trap(p, `ban ${theme} ${width}`, ban, 2)
        }
        await trap(p, `room settings ${section} ${theme} ${width}`, roomSettings)
        if (section !== 'Anonymous bans') await openRoomSettings()
      }
      const files = p.getByRole('button', { name: width <= 780 ? 'More' : 'Files of the desktop', exact: true })
      await files.click()
      if (width <= 780) await p.getByRole('menuitem', { name: 'Files of the desktop', exact: true }).click()
      await p.getByRole('dialog').getByRole('button', { name: 'Refresh', exact: true }).click(); await shot(p, 'files', theme, width); await trap(p, `files ${theme} ${width}`, files)
      const screenshot = p.getByRole('button', { name: 'Screenshot video', exact: true })
      await screenshot.click(); await expect(p.getByRole('button', { name: 'Crop', exact: true })).toBeEnabled()
      const cropArea = p.getByRole('group', { name: 'Crop area: arrow keys move, Shift and arrow keys resize' })
      await cropArea.focus()
      const cropBox = p.locator('.cropper-crop-box').last()
      const beforeCrop = await cropBox.boundingBox()
      await p.keyboard.press('ArrowRight'); await p.keyboard.press('Shift+ArrowDown')
      const afterCrop = await cropBox.boundingBox()
      assert(afterCrop.x > beforeCrop.x && afterCrop.height > beforeCrop.height, 'screenshot crop moves and resizes with keyboard')
      await shot(p, 'screenshot-dialog', theme, width)
      await p.getByRole('button', { name: 'Crop', exact: true }).click()
      await shot(p, 'upload-confirm', theme, width)
      await trap(p, `upload confirm ${theme} ${width}`, p.getByRole('button', { name: 'Crop', exact: true }).first(), 2)
      await trap(p, `screenshot ${theme} ${width}`, screenshot)
      await openPersonal()
      await p.getByRole('navigation', { name: 'Sections' }).getByRole('button', { name: 'My account', exact: true }).click()
      const avatar = p.getByRole('button', { name: 'Change avatar', exact: true }).last()
      await avatar.focus()
      const png = await p.evaluate(() => { const c = document.createElement('canvas'); c.width = c.height = 120; c.getContext('2d').fillRect(0, 0, 120, 120); return c.toDataURL().split(',')[1] })
      await p.locator('input[type=file][accept="image/png,image/jpeg,image/webp"]').setInputFiles({ name: 'avatar.png', mimeType: 'image/png', buffer: Buffer.from(png, 'base64') })
      await expect(p.getByRole('button', { name: 'Crop', exact: true })).toBeEnabled()
      await p.getByRole('group', { name: 'Crop area: arrow keys move, Shift and arrow keys resize' }).focus()
      const avatarBox = p.locator('.cropper-crop-box').last(), beforeAvatar = await avatarBox.boundingBox()
      await p.keyboard.press('Shift+ArrowRight')
      const afterAvatar = await avatarBox.boundingBox()
      assert(afterAvatar.width > beforeAvatar.width && Math.abs(afterAvatar.width - afterAvatar.height) < 1, 'avatar keyboard resize remains square')
      await shot(p, 'avatar-cropper', theme, width); await trap(p, `avatar ${theme} ${width}`, avatar, 2)
      await p.getByRole('dialog').locator('input[maxlength="12"]').fill('Unsaved Alice')
      await expect.poll(() => p.locator('[aria-label="Chat preview"] [data-chat-name]').last().evaluate(el => el.style.getPropertyValue('--readable-name-colour'))).not.toBe('')
      const closeProfile = p.getByRole('dialog').getByRole('button', { name: 'Close', exact: true })
      await closeProfile.click(); await shot(p, 'discard-profile', theme, width)
      await trap(p, `discard profile ${theme} ${width}`, closeProfile, 2)
      await closeProfile.click(); await p.getByRole('button', { name: 'Discard', exact: true }).click()
      await expect(p.getByRole('dialog')).toHaveCount(0)
      await p.route('**/ux-image.png', route => route.fulfill({ contentType: 'image/png', body: Buffer.from(png, 'base64') }))
      h.send({ type: 'chat', message: { ...msg(20, bob, ''), type: 'image', mediaUrl: '/ux-image.png' } })
      const image = p.getByRole('button', { name: 'Open image', exact: true }); await image.click()
      await shot(p, 'media-preview', theme, width); await trap(p, `media ${theme} ${width}`, image)
      h.send({ type: 'kicked', reason: 'kicked' })
      await p.getByText('You have been kicked', { exact: true }).waitFor()
      await shot(p, 'kick', theme, width)
      await p.close()
    }
    assert.deepEqual(errors, [])
  } finally {
    fs.writeFileSync(`${output}/ux-results.json`, JSON.stringify({ results, keyboard, errors, contrast }, null, 2))
    await browser.close()
  }
  // Text over the stream has no single background to measure against.
  const failures = contrast.filter(row => row.ratio < row.required && !row.screen.startsWith('room-overlay'))
  assert.equal(failures.length, 0, JSON.stringify(failures.map(({ screen, theme, width, text, ratio }) => ({ screen, theme, width, text, ratio }))))
  assert.equal(results.filter(row => row.result !== 'captured').length, 0, 'no page overflow')
  console.log(`Passed: ${results.length} screenshots; ${keyboard.length} keyboard traversals; ${contrast.length} text contrast measurements.`)
}
run().catch(e => { console.error(e); process.exitCode = 1 })
