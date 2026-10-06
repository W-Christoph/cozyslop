// Optional native clipboard coverage: point PLAYWRIGHT_MODULE at an existing
// Playwright installation and CHAT_TEST_URL at Vite. No clipboard read API.
const { chromium, firefox, expect } = require(process.env.PLAYWRIGHT_MODULE || 'playwright/test')
const assert = require('node:assert/strict')
const base = process.env.CHAT_TEST_URL || 'http://127.0.0.1:5178'
const fixture = `<!doctype html><div id="app"></div><textarea id="source"></textarea>
<script type="module">
import { render } from 'PREACT_MODULE'
import { signal } from 'SIGNALS_MODULE'
import { useRef } from 'HOOKS_MODULE'
import { RemoteScreen } from '/src/components/room/RemoteScreen.tsx'
import { MobileRemoteControls } from '/src/components/room/MobileRemoteControls.tsx'
import { RoomContext } from '/src/components/room/RoomContext.ts'
import { preferences, updatePreferences } from 'STATE_MODULE'
import '/src/styles/tokens.css'
import '/src/styles/base.css'
window.sent = []; window.keys = []
window.store = { isHost: signal(true), video: signal('disconnected'), paused: signal(false), neko: {
 screen: { width: 1280, height: 720 }, on() { return () => {} },
 keyDown(k) { keys.push(['down', k]) }, keyUp(k) { keys.push(['up', k]) },
 paste(t) { sent.push(t) }, move() {}, buttonDown() {}, buttonUp() {}, releaseButtons() {}, scroll() {}
} }
window.prefs = preferences; window.updatePreferences = updatePreferences
// Use Preact's vnode factory so refs and signals follow the production path.
import { h } from 'PREACT_MODULE'
function Fixture() { const video = useRef(null), pointer = useRef({ x: 0, y: 0 });
 return h(RoomContext.Provider, { value: store }, h('div', { style: 'width:800px;height:450px' },
 h(RemoteScreen, { mobile: false, video, pointer, onPlaybackBlocked() {} })), h(MobileRemoteControls, { pointer })) }
render(h(Fixture), document.getElementById('app'))
</script>`

async function run(name, browserType) {
  const browser = await browserType.launch({ headless: true })
  try {
    const page = await browser.newPage()
    const errors = []
    page.on('pageerror', e => errors.push(e.message))
    const [contextSource, screenSource, stateSource, pasteSource] = await Promise.all([
      '/src/components/room/RoomContext.ts', '/src/components/room/RemoteScreen.tsx', '/src/app/state.ts', '/src/components/room/useDesktopPaste.tsx',
    ].map(path => fetch(base + path).then(response => response.text())))
    const html = fixture.replaceAll('PREACT_MODULE', contextSource.match(/"([^"]*\/preact\.js[^"]*)"/)[1])
      .replaceAll('STATE_MODULE', pasteSource.match(/"([^"]*\/app\/state\.ts[^"]*)"/)[1])
      .replaceAll('HOOKS_MODULE', screenSource.match(/"([^"]*\/preact_hooks\.js[^"]*)"/)[1])
      .replaceAll('SIGNALS_MODULE', stateSource.match(/"([^"]*\/@preact_signals\.js[^"]*)"/)[1])
    await page.route('**/paste-fixture', route => route.fulfill({ contentType: 'text/html', body: html }))
    await page.goto(`${base}/paste-fixture`)
    const screen = page.getByLabel('Remote desktop', { exact: true })
    await expect(screen).toBeVisible().catch(e => { console.error(errors); throw e })
    const copy = async text => {
      await page.locator('#source').fill(text)
      await page.locator('#source').selectText()
      await page.keyboard.press('Control+C')
      await screen.focus()
    }
    const text = ' \tprivate text\nsecond line  '
    await copy(text)
    await page.keyboard.press('Control+V')
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('pre')).toHaveText(text, { useInnerText: false })
    await expect(page.getByRole('button', { name: 'Paste', exact: true })).toBeFocused()
    assert.deepEqual(await page.evaluate(() => sent), [])
    assert(!(await page.evaluate(() => keys)).some(([_, k]) => k === 0x76 || k === 0x56), 'V never goes to the desktop')
    const before = await page.evaluate(() => keys.length)
    await page.keyboard.press('x')
    await screen.dispatchEvent('keydown', { key: 'x', keyCode: 88, bubbles: true })
    assert.equal(await page.evaluate(() => keys.length), before, 'dialog blocks desktop keys')
    await page.keyboard.press('Escape')
    await expect(dialog).toHaveCount(0)
    await expect(screen).toBeFocused()
    assert.deepEqual(await page.evaluate(() => sent), [])
    await page.keyboard.press('Control+V')
    await expect(dialog).toBeVisible()
    await page.getByRole('button', { name: 'Cancel', exact: true }).click()
    await expect(screen).toBeFocused()
    const long = ' \t\n' + '😀'.repeat(2100) + '\nlast '
    await copy(long)
    await page.keyboard.press('Control+V')
    await expect(dialog).toBeVisible()
    await expect(dialog.locator('pre')).toHaveText(' \t\n' + '😀'.repeat(1997), { useInnerText: false })
    await expect(dialog).toContainText('… 109 more characters')
    await page.getByLabel("Don't ask again").check()
    await page.getByRole('button', { name: 'Paste', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(screen).toBeFocused()
    assert.deepEqual(await page.evaluate(() => sent), [long])
    assert.equal(await page.evaluate(() => JSON.parse(localStorage.getItem('preferences')).askBeforePaste), false)
    await copy(text)
    await page.keyboard.press('Control+V')
    await expect(dialog).toHaveCount(0)
    assert.deepEqual(await page.evaluate(() => sent), [long, text])
    await page.evaluate(() => updatePreferences({ askBeforePaste: true }))
    await page.getByLabel('Remote keyboard', { exact: true }).focus()
    await page.keyboard.type('A')
    assert.equal((await page.evaluate(() => sent)).at(-1), 'A', 'mobile typing stays immediate')
    await page.keyboard.press('Control+V')
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByLabel('Remote keyboard', { exact: true })).toBeFocused()
    await page.evaluate(() => { store.isHost.value = false })
    await screen.focus()
    const count = await page.evaluate(() => sent.length)
    await page.keyboard.press('Control+V')
    await expect(dialog).toHaveCount(0)
    assert.equal(await page.evaluate(() => sent.length), count)
    assert.deepEqual(errors, [])
    console.log(name + ': native paste, preview, accept/cancel, focus, persistence, mobile and non-host passed')
  } finally { await browser.close() }
}
(async () => { await run('Chromium', chromium); await run('Firefox', firefox) })().catch(e => { console.error(e); process.exitCode = 1 })
