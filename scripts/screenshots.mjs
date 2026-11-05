// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds 38 places, a year of visits, plans and lists):
//
//   DATA_DIR=/tmp/places-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Maya PORT=18306 ./pocket-places
//   CDP=http://127.0.0.1:19271 BASE=http://127.0.0.1:18306 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                  # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18306'
const shots = process.env.SHOTS !== '0'
const dir = new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500, frozen = true) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  if (frozen) await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' })
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots) await page.screenshot({ path: dir + name + '.png' }) }

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 2500)
  assert.ok((await page.content()).includes('How was it?'), 'the overview asks how a visit was')
  await shot(page, 'desktop-overview')
  await go(page, '/places/9', 2500)
  assert.ok((await page.content()).includes('Kafeterija'), 'a place shows its page')
  await shot(page, 'desktop-place')
  await go(page, '/map', 3500)
  await shot(page, 'desktop-map')
  await go(page, '/lists/5', 3000)
  await shot(page, 'desktop-list')
  await go(page, '/places?status=want')
  await shot(page, 'desktop-places')
  await go(page, '/visits')
  await shot(page, 'desktop-visits')
  for (const p of ['/places', '/places/4', '/places/17', '/lists', '/lists/1', '/settings/apps', '/settings/notifications', '/settings/general', '/settings/data', '/settings/ai', '/settings/about']) await go(page, p, 500)
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 2000)
  await shot(page, 'phone-overview')
  await go(page, '/places/4', 2500)
  await shot(page, 'phone-place')
  await go(page, '/map', 3500)
  await shot(page, 'phone-map')
  await ctx.close()
}
{
  const { ctx, page } = await session({ phone: true, dark: true })
  await go(page, '/places/9', 2500)
  await page.evaluate(() => window.scrollTo(0, 0))
  await shot(page, 'phone-place-dark')
  await go(page, '/lists/1', 2500)
  await shot(page, 'phone-list-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) {
  console.error('browser errors:\n' + errors.join('\n'))
  process.exit(1)
}
console.log('ok')
