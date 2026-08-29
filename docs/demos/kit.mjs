// The recording kit: everything a plot needs that is not the plot itself.
//
// Plots live in a separate repository (see README). They import from here, so
// this file — not the plot — owns browser setup, the on-screen overlays, the
// cursor, and the review artefacts a take produces.
//
// Two rules this kit exists to enforce:
//
//   1. Wait on conditions, never on sleeps. `until()` polls real backend state,
//      so a slow build or LLM call stretches the take instead of desyncing it.
//   2. Address controls through SEL. A raw selector in a plot is coupling this
//      repo cannot see; selector-contract.test.js pins every SEL entry.

import { chromium } from 'playwright'
import { execFileSync } from 'node:child_process'
import { mkdirSync, writeFileSync, existsSync } from 'node:fs'
import { join } from 'node:path'
import { FIXTURE } from './fixture.mjs'

const CHROMIUM =
  process.env.TEXERE_CHROMIUM ??
  '/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome'
const TEXERE_ROOT = new URL('../../', import.meta.url).pathname

const mmss = (ms) => {
  const s = Math.floor(ms / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// Which texere produced this recording. Stamped into the metadata so a video
// can always be traced back to a commit — the traceability a submodule would
// give, without pinning the plots to a stale UI.
function texereCommit() {
  try {
    const git = (...a) => execFileSync('git', ['-C', TEXERE_ROOT, ...a]).toString().trim()
    const sha = git('rev-parse', '--short', 'HEAD')
    const dirty = git('status', '--porcelain').length > 0
    return dirty ? `${sha} (dirty)` : sha
  } catch {
    return 'unknown'
  }
}

const overlays = (title, subtitle) => {
  // Injected at document-start. Guarded and retried: at that point neither
  // <head> nor documentElement is guaranteed to exist, and an uncaught throw
  // aborts the whole init script — which silently removes every overlay from
  // the take while the run still reports success.
  const paint = () => {
    if (document.getElementById('__bg')) return
    const style = document.createElement('style')
    style.id = '__bg'
    style.textContent = 'html,body{background:#0d1520!important}'
    const host = document.head || document.documentElement
    if (host) host.appendChild(style)
  }
  try { paint() } catch { /* retried below */ }

  addEventListener('DOMContentLoaded', () => {
    paint()
    const card = document.createElement('div')
    card.id = '__title'
    card.style.cssText =
      'position:fixed;inset:0;background:#0d1520;z-index:2147483645;display:flex;' +
      'flex-direction:column;align-items:center;justify-content:center;gap:14px;' +
      'font-family:Nunito,system-ui,sans-serif;transition:opacity .7s'
    card.innerHTML =
      `<div style="font-size:46px;font-weight:800;color:#e8eef8;letter-spacing:-.5px">${title}</div>` +
      `<div style="font-size:20px;color:#7fa8e8;font-weight:600">${subtitle}</div>`
    document.body.appendChild(card)

    const cur = document.createElement('div')
    cur.id = '__cur'
    cur.style.cssText =
      'position:fixed;width:22px;height:22px;border-radius:50%;background:rgba(255,255,255,.92);' +
      'box-shadow:0 0 0 3px rgba(60,140,255,.85),0 2px 10px rgba(0,0,0,.5);z-index:2147483647;' +
      'pointer-events:none;left:-60px;top:-60px;transition:transform .08s'
    document.body.appendChild(cur)

    const cap = document.createElement('div')
    cap.id = '__cap'
    cap.style.cssText =
      'position:fixed;left:50%;transform:translateX(-50%);bottom:34px;' +
      'font:600 21px/1.45 Nunito,system-ui,sans-serif;color:#fff;background:rgba(12,18,30,.94);' +
      'padding:13px 26px;border-radius:10px;z-index:2147483646;pointer-events:none;opacity:0;' +
      'transition:opacity .3s;border:1px solid rgba(90,150,255,.45);max-width:82vw;text-align:center'
    document.body.appendChild(cap)

    addEventListener('mousemove', (e) => {
      cur.style.left = e.clientX - 11 + 'px'
      cur.style.top = e.clientY - 11 + 'px'
    })
    addEventListener('mousedown', () => (cur.style.transform = 'scale(.65)'))
    addEventListener('mouseup', () => (cur.style.transform = 'scale(1)'))
  })
}

export async function session({
  title = 'Texere',
  subtitle = '',
  slug = 'session',
  out = process.env.TEXERE_DEMO_OUT,
  url = process.env.TEXERE_DEMO_URL ?? 'http://localhost:34115',
  fixture = FIXTURE,
} = {}) {
  if (!out) throw new Error('set TEXERE_DEMO_OUT to the directory recordings should land in')
  if (!existsSync(join(fixture, 'study-doc.md'))) {
    throw new Error(`fixture missing at ${fixture}\nRun: npm run seed, then restart the app`)
  }
  mkdirSync(join(out, 'frames'), { recursive: true })

  const browser = await chromium.launch({
    executablePath: CHROMIUM,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    recordVideo: { dir: join(out, slug), size: { width: 1440, height: 900 } },
  })
  const started = Date.now()
  const page = await ctx.newPage()

  const pageErrors = []
  page.on('pageerror', (e) => pageErrors.push(String(e).slice(0, 160)))

  await page.addInitScript(overlays, title, subtitle)
  await page.goto(url, { waitUntil: 'domcontentloaded' })
  await page.waitForFunction(() => window.go?.main?.App, null, { timeout: 25000 })

  // An app that booted before `npm run seed` renders only "Open collection",
  // whose native dialog a browser cannot reach. Say so, rather than letting a
  // selector time out 30 seconds later with a CSS class for a message.
  const open = await page.evaluate(() => window.go.main.App.GetInitialCollectionDir())
  if (open !== fixture) {
    await browser.close()
    throw new Error(
      `the app is on ${open}, not the fixture ${fixture}\n` +
        'Run: npm run seed, then restart the app (make dev-demo).',
    )
  }

  const beats = []
  const notes = []
  let cursor = { x: 720, y: 450 }
  const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2)

  const t = {
    page,
    fixture,

    // Playwright's {steps} interpolates linearly, which reads as machine
    // motion. Ease, and start from wherever the pointer actually is.
    async glide(x, y, steps = 26) {
      const from = { ...cursor }
      for (let i = 1; i <= steps; i++) {
        const k = ease(i / steps)
        await page.mouse.move(from.x + (x - from.x) * k, from.y + (y - from.y) * k)
        await page.waitForTimeout(9)
      }
      cursor = { x, y }
    },

    async click(selector) {
      const el = page.locator(selector).first()
      await el.scrollIntoViewIfNeeded().catch(() => {})
      const box = await el.boundingBox()
      if (!box) throw new Error(`no element for ${selector}`)
      await t.glide(box.x + box.width / 2, box.y + box.height / 2)
      await page.waitForTimeout(260)
      await page.mouse.down()
      await page.waitForTimeout(80)
      await page.mouse.up()
    },

    type: (text, delay = 85) => page.keyboard.type(text, { delay }),
    press: (key) => page.keyboard.press(key),
    wait: (ms) => page.waitForTimeout(ms),
    call: (fn, ...args) =>
      page.evaluate(([f, a]) => window.go.main.App[f](...a), [fn, args]),

    async say(text, hold = 2200) {
      await page.evaluate((s) => {
        const c = document.getElementById('__cap')
        c.textContent = s
        c.style.opacity = '1'
      }, text)
      await page.waitForTimeout(hold)
    },
    clear: () =>
      page.evaluate(() => {
        const c = document.getElementById('__cap')
        if (c) c.style.opacity = '0'
      }),

    // Wait on real backend state. `until(c => c.process > 0)` polls
    // GetStudyCounts; pass a string to wait on a selector instead.
    async until(pred, { timeout = 15000, label } = {}) {
      if (typeof pred === 'string') {
        await page.waitForSelector(pred, { timeout })
      } else {
        // waitForFunction takes a string expression and awaits a promise
        // result, so the predicate can be handed straight to .then() in the
        // page. No eval, and the plot writes a plain arrow function.
        await page.waitForFunction(
          `window.go.main.App.GetStudyCounts().then(${pred.toString()})`,
          null,
          { timeout },
        )
      }
      notes.push(label ?? `until ${pred.toString().slice(0, 60)}`)
    },

    // Select a phrase inside a container, as a user would drag over it. The
    // reader reads window.getSelection(), so this must be a real Range.
    async selectText(phrase, container) {
      const got = await page.evaluate(
        ([sel, text]) => {
          const el = document.querySelector(sel)
          if (!el) return { ok: false, why: `no ${sel}` }
          const walk = document.createTreeWalker(el, NodeFilter.SHOW_TEXT)
          let n
          while ((n = walk.nextNode())) {
            const i = n.textContent.indexOf(text)
            if (i >= 0) {
              const r = document.createRange()
              r.setStart(n, i)
              r.setEnd(n, i + text.length)
              const s = window.getSelection()
              s.removeAllRanges()
              s.addRange(r)
              return { ok: true, text: s.toString() }
            }
          }
          return { ok: false, why: 'phrase not found in ' + sel }
        },
        [container, phrase],
      )
      if (!got.ok) throw new Error(`selectText: ${got.why}`)
      return got
    },

    // A named unit of the recording. Timestamps become YouTube chapters and a
    // frame is captured for the contact sheet, so a take can be reviewed
    // without watching it.
    async beat(name, fn) {
      const at = Date.now() - started
      const idx = String(beats.length + 1).padStart(2, '0')
      beats.push({ name, at })
      console.log(`  [${mmss(at)}] ${name}`)
      await fn(t)
      await page.screenshot({ path: join(out, 'frames', `${idx}.png`) }).catch(() => {})
    },

    note: (line) => {
      notes.push(line)
      console.log('    ·', line)
    },

    // Dissolve the title card once the app is actually up.
    async open(hold = 2100) {
      await page.waitForSelector('button[title^="Study"]', { timeout: 20000 })
      await page.waitForTimeout(hold)
      await page.evaluate(() => {
        const c = document.getElementById('__title')
        if (c) c.style.opacity = '0'
      })
      await page.waitForTimeout(900)
      await page.evaluate(() => document.getElementById('__title')?.remove())
      await page.waitForTimeout(600)
    },

    async finish() {
      await t.clear()
      await page.waitForTimeout(800)
      await ctx.close()
      await browser.close()

      const commit = texereCommit()
      writeFileSync(
        join(out, `${slug}.chapters.txt`),
        beats.map((b) => `${mmss(b.at)} ${b.name}`).join('\n') + '\n',
      )
      writeFileSync(
        join(out, `${slug}.meta.txt`),
        [
          `title:    ${title}`,
          `subtitle: ${subtitle}`,
          `recorded: ${new Date().toISOString()}`,
          `texere:   ${commit}`,
          `fixture:  ${fixture}`,
          `beats:    ${beats.length}`,
          '',
          'assertions:',
          ...notes.map((n) => `  - ${n}`),
          '',
          'page errors:',
          ...(pageErrors.length ? pageErrors.map((e) => `  - ${e}`) : ['  (none)']),
        ].join('\n') + '\n',
      )
      console.log(`\n  texere ${commit} · ${beats.length} beats`)
      console.log(`  video    ${join(out, slug)}`)
      console.log(`  chapters ${join(out, slug)}.chapters.txt`)
      console.log(`  frames   ${join(out, 'frames')}`)
      if (pageErrors.length) console.log(`  ${pageErrors.length} page error(s) — see meta`)
      return { beats, notes, pageErrors, commit }
    },
  }

  return t
}
