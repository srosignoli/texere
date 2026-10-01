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
import { mkdirSync, writeFileSync, existsSync, rmSync } from 'node:fs'
import { join } from 'node:path'
import { FIXTURE } from './fixture.mjs'
import { SEL } from './selectors.mjs'
import { concatScript, clipTime, maxGap, assertEpochSeconds } from './screencast.mjs'
import { SHORTCUT_GROUPS } from '../../frontend/src/lib/shortcuts.js'

// Narration cites keybindings, and a caption that names the wrong key is worse
// than one that names none. Read them from the app's own registry — the same
// data the F1 panel renders and shortcuts.test.js guards — so a rebinding
// changes the caption instead of making it lie.
//
// The registry covers modified combos only: shortcuts.test.js treats unmodified
// keys as "contextual, out of scope", so Space and the 1-4 grades are not in
// here and cannot be looked up. State those literally in the plot, and know
// they can drift silently.
const SHORTCUTS = new Map(
  SHORTCUT_GROUPS.flatMap((g) => g.items.map((i) => [i.label, i.keys.join('+')])),
)

// A registry combo as the keyboard actually sends it. See t.shortcut below for
// why the case of the final key matters; exported so a plot pressing a combo
// the registry does not carry can reach the same normalisation.
export function asPhysicalCombo(combo) {
  const parts = combo.replace('Ctrl/Cmd', 'Control').split('+')
  const last = parts[parts.length - 1]
  if (last.length === 1 && /[a-z]/i.test(last)) {
    parts[parts.length - 1] = parts.includes('Shift') ? last.toUpperCase() : last.toLowerCase()
  }
  return parts.join('+')
}

export function keysFor(label) {
  const combo = SHORTCUTS.get(label)
  if (!combo) {
    throw new Error(
      `no documented shortcut labelled "${label}".\n` +
        'Modified combos live in frontend/src/lib/shortcuts.js; unmodified keys ' +
        '(Space, 1-4) are contextual and deliberately not in the registry.',
    )
  }
  return combo
}

const CHROMIUM =
  process.env.TEXERE_CHROMIUM ??
  '/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome'

// The overview's edit script renders its cards with the same browser the takes
// use, and the tutorials repo cannot resolve playwright on its own.
export { chromium, CHROMIUM }
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

// addInitScript passes at most one argument, so title and subtitle arrive as
// one object rather than two parameters.
const overlays = ({ title, subtitle, captions, titleCard }) => {
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
    if (titleCard) {
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
    }

    const cur = document.createElement('div')
    cur.id = '__cur'
    cur.style.cssText =
      'position:fixed;width:22px;height:22px;border-radius:50%;background:rgba(255,255,255,.92);' +
      'box-shadow:0 0 0 3px rgba(60,140,255,.85),0 2px 10px rgba(0,0,0,.5);z-index:2147483647;' +
      'pointer-events:none;left:-60px;top:-60px;transition:transform .08s'
    document.body.appendChild(cur)

    if (captions) {
      const cap = document.createElement('div')
      cap.id = '__cap'
      cap.style.cssText =
        'position:fixed;left:50%;transform:translateX(-50%);bottom:34px;' +
        'font:600 21px/1.45 Nunito,system-ui,sans-serif;color:#fff;background:rgba(12,18,30,.94);' +
        'padding:13px 26px;border-radius:10px;z-index:2147483646;pointer-events:none;opacity:0;' +
        'transition:opacity .3s;border:1px solid rgba(90,150,255,.45);max-width:82vw;text-align:center'
      document.body.appendChild(cap)
    }

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
  capture = 'video',
  scale = 1,
  captions = true,
  titleCard = true,
} = {}) {
  if (!out) throw new Error('set TEXERE_DEMO_OUT to the directory recordings should land in')
  if (!existsSync(join(fixture, 'study-doc.md'))) {
    throw new Error(`fixture missing at ${fixture}\nRun: npm run seed, then restart the app`)
  }
  mkdirSync(join(out, 'frames'), { recursive: true })

  if (!['video', 'screencast'].includes(capture)) throw new Error(`unknown capture "${capture}"`)
  // Playwright's recorder ignores deviceScaleFactor: it captures at 1x and pads
  // the larger frame with grey (measured, 2026-10-01). Only the screencast can
  // record a scaled page.
  if (capture === 'video' && scale !== 1) {
    throw new Error('capture "video" records at scale 1 only — use capture "screencast"')
  }

  const browser = await chromium.launch({
    executablePath: CHROMIUM,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  })
  const ctx = await browser.newContext({
    viewport: { width: 1440, height: 900 },
    deviceScaleFactor: scale,
    ...(capture === 'video'
      ? { recordVideo: { dir: join(out, slug), size: { width: 1440, height: 900 } } }
      : {}),
  })
  const started = Date.now()
  const page = await ctx.newPage()

  // Screencast capture: JPEG frames at the page's real resolution, each
  // written as it arrives and acknowledged so CDP sends the next.
  const shots = []
  let cdp = null
  let clockError = null
  const shotDir = join(out, slug, 'screencast')
  if (capture === 'screencast') {
    rmSync(shotDir, { recursive: true, force: true }) // a previous take's frames must not join this one
    mkdirSync(shotDir, { recursive: true })
    cdp = await ctx.newCDPSession(page)
    cdp.on('Page.screencastFrame', (f) => {
      // Not thrown here: an exception inside a CDP event listener escapes the
      // plot's control flow. Held, and thrown by finish() before anything is
      // built from these timestamps.
      if (!shots.length) {
        try { assertEpochSeconds(f.metadata.timestamp, Date.now()) } catch (e) { clockError = e }
      }
      const file = join(shotDir, `${String(shots.length + 1).padStart(6, '0')}.jpg`)
      writeFileSync(file, Buffer.from(f.data, 'base64'))
      shots.push({ file, ts: f.metadata.timestamp })
      cdp.send('Page.screencastFrameAck', { sessionId: f.sessionId }).catch(() => {})
    })
    await cdp.send('Page.startScreencast', {
      format: 'jpeg',
      quality: 92,
      maxWidth: 1440 * scale,
      maxHeight: 900 * scale,
    })
  }

  const pageErrors = []
  page.on('pageerror', (e) => pageErrors.push(String(e).slice(0, 160)))

  await page.addInitScript(overlays, { title, subtitle, captions, titleCard })
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
  let current = null
  let cursor = { x: 720, y: 450 }
  const ease = (t) => (t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2)

  function writeClip(endWall) {
    if (clockError) throw clockError
    const list = join(out, slug, 'frames.ffconcat')
    writeFileSync(list, concatScript(shots, endWall / 1000))
    const mp4 = join(out, `${slug}.mp4`)
    execFileSync('ffmpeg', [
      '-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', list,
      '-vf', 'fps=60,format=yuv420p', '-c:v', 'libx264', '-preset', 'slow', '-crf', '12',
      '-movflags', '+faststart', mp4,
    ])
    const first = shots[0].ts
    const timeline = beats.map((b, i) => {
      const fromWall = started + b.at
      const toWall = i + 1 < beats.length ? started + beats[i + 1].at : endWall
      return {
        name: b.name,
        start: +clipTime(fromWall, first).toFixed(3),
        end: +clipTime(toWall, first).toFixed(3),
        maxGap: +maxGap(shots, fromWall / 1000, toWall / 1000).toFixed(3),
      }
    })
    writeFileSync(
      join(out, `${slug}.beats.json`),
      JSON.stringify({ fps: 60, scale, frames: shots.length, beats: timeline }, null, 2) + '\n',
    )
    console.log(`  clip     ${mp4}  (${shots.length} frames)`)
  }

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

    // Put the caret in the buffer, and PROVE it landed there.
    //
    // A click that misses the editor is silent in every way that matters: the
    // selector resolves, the click reports success, and the keystrokes that
    // follow go to whatever had focus instead — so a beat types into nothing
    // and the take records an unchanged file. That is not hypothetical; it is
    // what .cm-content's off-screen centre did before SEL.editor was repointed
    // at the scroller.
    //
    // .cm-focused is CodeMirror's own marker for "this editor has focus", so
    // asserting it is the difference between clicking at the editor and being
    // in it.
    async focusEditor() {
      await t.click(SEL.editor)
      await page.waitForFunction(() => document.querySelector('.cm-focused') !== null, null, {
        timeout: 5000,
      })
      notes.push('editor focused')
    },

    // 110ms, not 85. Review of the first six tutorials found the typing read
    // as machine-fast — text appearing quicker than a person could produce it
    // undercuts a screen recording, because the viewer stops believing they are
    // watching someone use the app. Queries and other short strings a viewer is
    // meant to READ as they appear should pass an even slower delay explicitly.
    type: (text, delay = 110) => page.keyboard.type(text, { delay }),
    press: (key) => page.keyboard.press(key),

    // The documented combo for an action, e.g. keys('Extract selection').
    keys: keysFor,

    // Perform an action the way the shortcut does, rather than by clicking.
    // A tutorial that only ever clicks teaches the slow path.
    //
    // The letter's CASE is not cosmetic. Playwright takes the key name
    // literally and does not derive case from the modifiers, so 'Control+K'
    // delivers { key: 'K', shiftKey: false } — an event no physical keyboard
    // can produce, because K without Shift reports 'k'. The registry spells
    // its combos with capitals for people to read, so pressing them verbatim
    // synthesises impossible events.
    //
    // That went unnoticed while every target was a CodeMirror keymap, which
    // matches case-insensitively. It surfaced on the omnibar, whose handler
    // compares e.key directly and accepts only lowercase 'k' for fuzzy search:
    // the panel simply never opened, and the beat timed out on a selector with
    // no hint that the keypress was the problem.
    //
    // So: lowercase a single-letter key when Shift is absent, uppercase it
    // when Shift is present. Both are then exactly what the hardware sends.
    async shortcut(label) {
      const combo = keysFor(label)
      await page.keyboard.press(asPhysicalCombo(combo))
      notes.push(`pressed ${combo} (${label})`)
    },

    // Import the document a beat is about, and refuse to do it quietly twice.
    //
    // A second import of the same file lands as "study-doc-2", which shows up
    // in the sidebar on camera and would otherwise reach a published video
    // unnoticed. The fixture is stale in that case: re-seed and restart.
    async importDoc(file) {
      const rv = await t.call('DemoImportTextSource', join(fixture, file))
      if (!rv?.id) throw new Error(`importDoc: ${file} did not import`)
      const wanted = file.replace(/\.[^.]+$/, '')
      if (rv.title !== wanted) {
        throw new Error(
          `"${file}" imported as "${rv.title}" — it is already in this fixture.\n` +
            'The take would show a duplicate in the sidebar. Re-seed ' +
            '(npm run seed) and restart the app before recording.',
        )
      }
      notes.push(`imported ${rv.id} → ${rv.refPath}`)
      return rv
    },

    // Import supporting documents off-camera. Texere opened on a real
    // collection shows a populated library; a workspace holding one file reads
    // as a demo rig. Call this before open(), while the title card still covers
    // the window.
    async primeLibrary(files) {
      // Idempotent: importing the same file twice yields "encoding-2" beside
      // "encoding", and a sidebar full of -2 suffixes is visible on camera.
      // Set dressing should never be the thing a viewer notices.
      const existing = new Set(
        ((await t.call('GetReadingList')) ?? []).map((r) => r.title),
      )
      let added = 0
      for (const f of files) {
        if (existing.has(f.replace(/\.[^.]+$/, ''))) continue
        const rv = await t.call('DemoImportTextSource', join(fixture, f))
        if (!rv?.id) throw new Error(`primeLibrary: ${f} did not import`)
        added++
      }
      notes.push(
        `primed library: ${added} imported, ${files.length - added} already present`,
      )
    },
    wait: (ms) => page.waitForTimeout(ms),
    call: (fn, ...args) =>
      page.evaluate(([f, a]) => window.go.main.App[f](...a), [fn, args]),

    async say(text, hold = 2200) {
      // Captions off: the line is the plot's script, not on-screen text. Keep
      // the hold so the take keeps its pacing for the edit to trim.
      if (!captions) {
        console.log(`    “${text}”`)
        await page.waitForTimeout(hold)
        return
      }
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

    // Capture the frame that represents this beat on the contact sheet.
    //
    // Call it at the moment the beat is *about*, which is rarely the end: a
    // beat that creates a card ends with an empty queue, so an end-of-beat
    // frame shows the aftermath and reads as if nothing happened. Beats that
    // do not call this fall back to an end-of-beat capture.
    async frame() {
      if (!current) return
      // One frame per beat: the contact sheet has one tile per beat, so a
      // second call overwrites the first at the same path. That is silent, and
      // it costs exactly the frame someone chose deliberately — say so in the
      // meta rather than letting the review packet quietly lose it.
      if (current.framed) {
        notes.push(
          `WARNING beat "${current.name}" called frame() more than once — ` +
            'only the last is kept; split the beat or drop a call',
        )
      }
      current.framed = true
      await page
        .screenshot({ path: join(out, 'frames', `${current.idx}.png`) })
        .catch(() => {})
    },

    // A named unit of the recording. Timestamps become YouTube chapters and a
    // frame goes to the contact sheet, so a take can be reviewed without
    // watching it.
    async beat(name, fn) {
      const at = Date.now() - started
      const idx = String(beats.length + 1).padStart(2, '0')
      current = { name, at, idx, framed: false }
      beats.push(current)
      console.log(`  [${mmss(at)}] ${name}`)
      await fn(t)
      if (!current.framed) {
        await page.screenshot({ path: join(out, 'frames', `${idx}.png`) }).catch(() => {})
      }
      current = null
    },

    note: (line) => {
      notes.push(line)
      console.log('    ·', line)
    },

    // Dissolve the title card once the app is actually up.
    //
    // The backdrop goes with it. It exists only to cover the white flash
    // between document-start and the app's first paint, and it carries
    // !important on html,body — which outranks style.css's own
    // `html { background-color: var(--surface) }`. Left in place it survives
    // the whole take, so anything TRANSPARENT keeps showing #0d1520 no matter
    // which theme is selected: the three columns stay dark while the editor
    // and the popover, which paint their own token backgrounds, turn light.
    // That is not a Chromium repaint problem, it is this stylesheet.
    async open(hold = 2100) {
      await page.waitForSelector('button[title^="Study"]', { timeout: 20000 })
      await page.waitForTimeout(hold)
      await page.evaluate(() => {
        const c = document.getElementById('__title')
        if (c) c.style.opacity = '0'
      })
      await page.waitForTimeout(900)
      await page.evaluate(() => {
        document.getElementById('__title')?.remove()
        document.getElementById('__bg')?.remove()
      })
      await page.waitForTimeout(600)
    },

    async finish() {
      await t.clear()
      await page.waitForTimeout(800)
      const endWall = Date.now()
      if (cdp) await cdp.send('Page.stopScreencast').catch(() => {})
      await ctx.close()
      await browser.close()
      if (capture === 'screencast') writeClip(endWall)

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
          `capture:  ${capture}${capture === 'screencast' ? ` @${scale}x, ${shots.length} frames` : ''}`,
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
