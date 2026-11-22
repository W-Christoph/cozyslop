// Pictures of the scenes of snapshots.html (src/snapshots/), as PNGs in
// ../snapshots/:
//
//   npm run snapshots                       every scene
//   npm run snapshots -- watch-ask          the scenes named
//   npm run snapshots -- --theme=light      another theme (in the file name)
//
// It starts Vite and opens the page in an installed Chrome or Edge; set
// CHROME_PATH to use another (e.g. from
// `npx @puppeteer/browsers install chrome-headless-shell@stable`).
import { existsSync, mkdirSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import puppeteer from 'puppeteer-core'
import { createServer } from 'vite'

const web = resolve(fileURLToPath(import.meta.url), '../..')
const out = resolve(web, '../snapshots')
const theme = process.argv.find((a) => a.startsWith('--theme='))?.slice('--theme='.length)
const names = process.argv.slice(2).filter((a) => !a.startsWith('--'))

function browserPath() {
  if (process.env.CHROME_PATH) return process.env.CHROME_PATH
  const env = process.env
  const candidates = {
    win32: [env.PROGRAMFILES, env['PROGRAMFILES(X86)'], env.LOCALAPPDATA].filter(Boolean).flatMap((dir) => [
      join(dir, 'Google/Chrome/Application/chrome.exe'), join(dir, 'Microsoft/Edge/Application/msedge.exe')]),
    darwin: ['/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', '/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge',
      '/Applications/Chromium.app/Contents/MacOS/Chromium'],
    linux: ['/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/usr/bin/microsoft-edge'],
  }[process.platform] ?? []
  return candidates.find((p) => existsSync(p))
}

const executablePath = browserPath()
if (!executablePath) {
  console.error('No Chrome or Edge found. Set CHROME_PATH, e.g. after `npx @puppeteer/browsers install chrome-headless-shell@stable`.')
  process.exit(1)
}

const server = await createServer({ root: web, logLevel: 'error', server: { port: 5199, host: '127.0.0.1' } })
await server.listen()
const browser = await puppeteer.launch({ executablePath, headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage'] })
let failed = false
try {
  const page = await browser.newPage()
  await page.setViewport({ width: 1600, height: 1000, deviceScaleFactor: 2 })
  page.on('pageerror', (e) => { failed = true; console.error('page error:', e.message) })
  const url = new URL('snapshots.html', server.resolvedUrls.local[0])
  if (theme) url.searchParams.set('theme', theme)
  await page.goto(url.href, { waitUntil: 'networkidle0' })
  await page.evaluate(() => document.fonts.ready)
  mkdirSync(out, { recursive: true })
  const shot = []
  for (const element of await page.$$('[data-snapshot]')) {
    const name = await element.evaluate((e) => e.dataset.snapshot)
    if (names.length && !names.includes(name)) continue
    const file = join(out, `${name}${theme ? `.${theme}` : ''}.png`)
    await element.screenshot({ path: file })
    shot.push(file)
  }
  // Windows: each alone, the whole screen.
  const windows = await page.$$eval('[data-window]', (links) => links.map((a) => ({
    name: a.dataset.window, width: Number(a.dataset.width), height: Number(a.dataset.height) })))
  for (const w of windows) {
    if (names.length && !names.includes(w.name)) continue
    url.searchParams.set('scene', w.name)
    await page.setViewport({ width: w.width, height: w.height, deviceScaleFactor: 2 })
    await page.goto(url.href, { waitUntil: 'networkidle0' })
    await page.evaluate(() => document.fonts.ready)
    await new Promise((done) => setTimeout(done, 300)) // the window fades in
    const file = join(out, `${w.name}${theme ? `.${theme}` : ''}.png`)
    await page.screenshot({ path: file })
    shot.push(file)
  }
  for (const name of names.filter((n) => !shot.some((f) => f.includes(n)))) {
    failed = true
    console.error(`No scene called ${name}.`)
  }
  console.log(shot.join('\n'))
} finally {
  await browser.close()
  await server.close()
}
process.exit(failed ? 1 : 0)
