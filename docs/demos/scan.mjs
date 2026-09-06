// The fixture's scanned page: handwritten notes with a hand-drawn diagram.
//
// The scan cell ("+ Scan") is the notebook's route from paper to N4L: it
// imports a picture and hands it to a multimodal model under TRANSCRIBE_SYSTEM,
// which asks for faithful text AND for any diagram to come back as a fenced
// mermaid block. Demonstrating that needs a page that genuinely looks handmade
// — printed text would prove nothing, since OCR of clean type is not the claim.
//
// Generated rather than committed, like the rest of the fixture: seeding
// deletes and recreates the collection, so anything a take needs must be
// reproducible from this repo alone.
//
// Three decisions worth recording.
//
// THE HANDWRITING IS A REAL FONT, not a stroke font. Tutorial 14 draws block
// capitals from lib-strokefont.mjs because it needs strokes on a canvas; here
// the page only has to LOOK written, and a casual handwriting face does that
// far better than anything a polyline generator produces. The stack falls back
// through several faces because it depends on what is installed — on this
// machine "Ink Free" and "Bradley Hand ITC" are both present.
//
// THE DIAGRAM IS DRAWN ROUGH, from a seeded PRNG so the fixture is identical on
// every regeneration. Straight SVG lines and exact ellipses read as a computer
// drawing, and a computer drawing is not what this cell is for: the model would
// be transcribing something no one drew by hand.
//
// THE PAGE IS SAVED SIDEWAYS. A scan fed into the machine the wrong way round
// is the ordinary case, and bakeImage bakes cell.rotation into what it sends —
// so the tutorial can straighten it with the cell's own Rotate control and the
// model receives an upright page. A demonstration of a control that matters,
// for free.
//
// Both a PDF and a PNG come out. The cell can only ingest an image
// (ImportImage filters to *.jpg;*.jpeg;*.png;*.gif;*.webp), so the take uses
// the PNG; the PDF is the same page for anyone who wants the document itself.

import { chromium } from 'playwright'

const CHROMIUM =
  process.env.TEXERE_CHROMIUM ??
  '/home/simone/.cache/ms-playwright/chromium-1234/chrome-linux64/chrome'

// The page, in CSS pixels. Rendered at deviceScaleFactor 2 and then rotated,
// which gives a long edge of about 1600px — just over bake.js's MAX_EDGE of
// 1568, so the model receives essentially all the detail that survives its own
// downscale, and no more.
const W = 620
const H = 726

// A small deterministic PRNG (mulberry32). The fixture must be byte-identical
// on every seed, so nothing here may use Math.random.
function rng(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

const rand = rng(0x5ca4)
const jitter = (amount) => (rand() - 0.5) * 2 * amount

// A line as a person draws it: sampled along its length, pushed off course
// perpendicular to it, and never quite starting or ending where intended.
function roughLine(x1, y1, x2, y2, { wobble = 1.6, steps = 10 } = {}) {
  const dx = x2 - x1
  const dy = y2 - y1
  const len = Math.hypot(dx, dy) || 1
  const nx = -dy / len
  const ny = dx / len
  const pts = []
  for (let i = 0; i <= steps; i++) {
    const t = i / steps
    // Deviation is largest in the middle of a stroke and smallest at the ends,
    // where the pen is under most control.
    const bow = Math.sin(t * Math.PI) * wobble
    const k = jitter(bow)
    pts.push([x1 + dx * t + nx * k, y1 + dy * t + ny * k])
  }
  return pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')
}

// Two passes over the same line, which is what makes a sketch look sketched.
function stroke(d, { width = 2, opacity = 0.92 } = {}) {
  return `<path d="${d}" fill="none" stroke="var(--ink)" stroke-width="${width}" stroke-linecap="round" stroke-linejoin="round" opacity="${opacity}"/>`
}
function roughStroke(x1, y1, x2, y2, opts = {}) {
  return (
    stroke(roughLine(x1, y1, x2, y2, opts), { width: 2 }) +
    stroke(roughLine(x1, y1, x2, y2, opts), { width: 1.4, opacity: 0.5 })
  )
}

// An ellipse that overshoots its start, the way a pen does when you loop back
// round to where you began.
function roughEllipse(cx, cy, rx, ry) {
  const steps = 42
  const overshoot = 0.22 + rand() * 0.16 // radians past the start
  const start = rand() * Math.PI * 2
  const pts = []
  for (let i = 0; i <= steps; i++) {
    const a = start + (i / steps) * (Math.PI * 2 + overshoot)
    const w = 1 + jitter(0.022)
    pts.push([cx + Math.cos(a) * rx * w, cy + Math.sin(a) * ry * w])
  }
  const d = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')
  return stroke(d, { width: 2 }) + stroke(d, { width: 1.2, opacity: 0.45 })
}

// An arrowhead as two short strokes off the tip, at the angle of travel.
function arrowHead(x, y, angle) {
  const L = 11
  const spread = 0.42
  return (
    roughStroke(x, y, x - Math.cos(angle - spread) * L, y - Math.sin(angle - spread) * L, { wobble: 0.6, steps: 4 }) +
    roughStroke(x, y, x - Math.cos(angle + spread) * L, y - Math.sin(angle + spread) * L, { wobble: 0.6, steps: 4 })
  )
}

// A straight-ish arrow between two points, stopping short of the target so the
// head does not sit on top of a state's outline.
function arrow(x1, y1, x2, y2, opts = {}) {
  const a = Math.atan2(y2 - y1, x2 - x1)
  return roughStroke(x1, y1, x2, y2, opts) + arrowHead(x2, y2, a)
}

// A curved arrow, for the loop back from Relearning. Sampled off a quadratic
// so the wobble applies to a curve rather than a chord.
function curvedArrow(x1, y1, cx, cy, x2, y2) {
  const steps = 16
  const pts = []
  for (let i = 0; i <= steps; i++) {
    const t = i / steps
    const mt = 1 - t
    pts.push([
      mt * mt * x1 + 2 * mt * t * cx + t * t * x2 + jitter(1.1),
      mt * mt * y1 + 2 * mt * t * cy + t * t * y2 + jitter(1.1),
    ])
  }
  const d = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')
  const [px, py] = pts[steps - 1]
  const a = Math.atan2(y2 - py, x2 - px)
  return stroke(d, { width: 2 }) + stroke(d, { width: 1.2, opacity: 0.45 }) + arrowHead(x2, y2, a)
}

// A self-loop above a state.
function selfLoop(cx, cy, r) {
  const x1 = cx - r * 0.5
  const x2 = cx + r * 0.5
  const top = cy - r - 34
  const steps = 20
  const pts = []
  for (let i = 0; i <= steps; i++) {
    const t = i / steps
    const mt = 1 - t
    pts.push([
      mt * mt * x1 + 2 * mt * t * cx + t * t * x2 + jitter(1.0),
      mt * mt * (cy - r) + 2 * mt * t * top + t * t * (cy - r) + jitter(1.0),
    ])
  }
  const d = pts.map((p, i) => `${i ? 'L' : 'M'}${p[0].toFixed(1)},${p[1].toFixed(1)}`).join('')
  return stroke(d, { width: 2 }) + stroke(d, { width: 1.2, opacity: 0.45 }) + arrowHead(x2, cy - r, 1.9)
}

// The state machine: a card's life in a spaced-repetition schedule. Chosen
// because it is exactly what a student sketches in a margin, and because it is
// unambiguously a state machine — which is what the mermaid half of the prompt
// is for.
function diagram() {
  const states = [
    { id: 'new', label: 'NEW', x: 96, y: 96, rx: 54, ry: 30 },
    { id: 'learn', label: 'LEARNING', x: 292, y: 96, rx: 74, ry: 32 },
    { id: 'review', label: 'REVIEW', x: 292, y: 246, rx: 64, ry: 31 },
    { id: 'relearn', label: 'RELEARNING', x: 96, y: 246, rx: 78, ry: 33 },
  ]
  const by = Object.fromEntries(states.map((s) => [s.id, s]))

  let svg = ''
  for (const s of states) svg += roughEllipse(s.x, s.y, s.rx, s.ry)

  // NEW -> LEARNING
  svg += arrow(by.new.x + by.new.rx + 4, 96, by.learn.x - by.learn.rx - 6, 96)
  // LEARNING -> REVIEW
  svg += arrow(292, by.learn.y + by.learn.ry + 4, 292, by.review.y - by.review.ry - 6)
  // LEARNING self-loop ("again")
  svg += selfLoop(by.learn.x, by.learn.y, by.learn.ry)
  // REVIEW -> RELEARNING (a lapse)
  svg += arrow(by.review.x - by.review.rx - 4, 246, by.relearn.x + by.relearn.rx + 6, 246)
  // RELEARNING -> REVIEW, curving under
  svg += curvedArrow(96, by.relearn.y + by.relearn.ry + 2, 190, 330, 292, by.review.y + by.review.ry + 4)

  const labels = [
    { t: 'good', x: 196, y: 84 },
    { t: 'again', x: 292, y: 26 },
    { t: 'good', x: 306, y: 176 },
    { t: 'AGAIN = lapse', x: 216, y: 216 },
    { t: 'good', x: 192, y: 344 },
  ]
  let text = ''
  for (const l of labels) {
    text += `<text class="lbl" x="${l.x}" y="${l.y}" transform="rotate(${jitter(2.2).toFixed(2)} ${l.x} ${l.y})">${l.t}</text>`
  }
  for (const s of states) {
    text += `<text class="st" x="${s.x}" y="${s.y + 6}" transform="rotate(${jitter(1.8).toFixed(2)} ${s.x} ${s.y})">${s.label}</text>`
  }

  return `<svg viewBox="0 0 420 380" width="420" height="380">${svg}${text}</svg>`
}

// Each line of prose gets its own small rotation, because handwriting does not
// sit square on a baseline.
function written(lines) {
  return lines
    .map((l) => {
      if (l === '') return '<div class="gap"></div>'
      return `<div class="ln" style="transform:rotate(${jitter(0.45).toFixed(2)}deg) translateX(${jitter(3).toFixed(1)}px)">${l}</div>`
    })
    .join('')
}

const HTML = `<style>
  /* Whatever casual hand is installed. On this machine the first two both
     resolve; the generic cursive is the floor. */
  :root {
    --ink: #1e3050;
    --hand: 'Ink Free', 'Bradley Hand ITC', 'Segoe Script', 'Lucida Handwriting', cursive;
  }
  * { box-sizing: border-box }
  body { margin: 0; width: ${W}px; height: ${H}px; background: #fbfaf4; }
  /* Faint ruling, as on any pad of paper. */
  .page {
    position: relative; width: 100%; height: 100%; padding: 40px 42px;
    background-image: repeating-linear-gradient(
      to bottom, transparent 0 30px, rgba(120,140,170,0.16) 30px 31px);
    color: var(--ink); font-family: var(--hand);
  }
  h1 { font-size: 27px; margin: 0 0 2px; font-weight: normal; transform: rotate(-0.5deg) }
  .rule { height: 2px; background: var(--ink); opacity: .5; width: 268px; margin: 2px 0 16px;
          transform: rotate(0.35deg); border-radius: 2px }
  .ln { font-size: 19px; line-height: 30px; }
  .gap { height: 14px }
  .fig { margin: 6px 0 4px -6px }
  svg { overflow: visible }
  .st { font-family: var(--hand); font-size: 16px; fill: var(--ink); text-anchor: middle }
  .lbl { font-family: var(--hand); font-size: 14px; fill: var(--ink); text-anchor: middle; opacity: .95 }
  .note { font-size: 17px; opacity: .95; margin-top: 8px }
</style>
<div class="page">
  <h1>Spaced repetition &mdash; card states</h1>
  <div class="rule"></div>
  ${written([
    'Every card is in exactly one state.',
    'The grade you give it decides where it goes next.',
  ])}
  <div class="fig">${diagram()}</div>
  ${written([
    '',
    'Only REVIEW grows the interval.',
    'A lapse sends it back &mdash; it does not reset',
    'the card to NEW.',
  ])}
</div>`

/**
 * Render the handwritten scan fixture.
 *
 * Writes `pngPath` (what the scan cell imports) and, when `pdfPath` is given,
 * the same page as a PDF. The PNG is saved ROTATED 90° clockwise, as if the
 * sheet went through the scanner sideways — the tutorial straightens it with
 * the cell's Rotate control before transcribing.
 */
export async function writeScanFixture(pngPath, pdfPath) {
  const browser = await chromium.launch({ executablePath: CHROMIUM, args: ['--no-sandbox'] })
  try {
    const page = await browser.newPage({
      viewport: { width: W, height: H },
      deviceScaleFactor: 2,
    })
    await page.setContent(HTML, { waitUntil: 'load' })
    // Give the webfont lookup a moment; a fallback that resolves late would
    // otherwise be captured mid-swap.
    await page.evaluate(() => document.fonts.ready)

    if (pdfPath) {
      await page.pdf({ path: pdfPath, width: `${W}px`, height: `${H}px`, printBackground: true })
    }

    const upright = await page.screenshot({ type: 'png' })

    // Rotate 90° clockwise by drawing it into a transposed canvas. Done in the
    // page rather than with an image library so the generator needs nothing
    // beyond the browser the recordings already use.
    const rotated = await page.evaluate(async (b64) => {
      const img = new Image()
      img.src = 'data:image/png;base64,' + b64
      await img.decode()
      const c = document.createElement('canvas')
      c.width = img.naturalHeight
      c.height = img.naturalWidth
      const ctx = c.getContext('2d')
      ctx.translate(c.width / 2, c.height / 2)
      ctx.rotate(Math.PI / 2)
      ctx.drawImage(img, -img.naturalWidth / 2, -img.naturalHeight / 2)
      return c.toDataURL('image/png').split(',')[1]
    }, upright.toString('base64'))

    const { writeFileSync } = await import('node:fs')
    writeFileSync(pngPath, Buffer.from(rotated, 'base64'))
  } finally {
    await browser.close()
  }
  return pngPath
}
