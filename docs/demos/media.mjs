// The fixture's image and video, generated rather than committed.
//
// The preview tutorial needs a note that references media, and a fixture is on
// camera: a test pattern or a stock photo would read as a demo rig. So both
// assets are the same figure — Ebbinghaus's forgetting curve, with one review
// part-way through — drawn from the same function the fixture's math describes,
// `R = e^(-t/S)`. The still is the figure; the clip is the figure being drawn.
//
// Generated, not committed, for the reason the whole fixture is: `npm run seed`
// deletes and rebuilds the collection directory, so anything the take needs
// must be reproducible from this repo alone.
//
// The video is written as raw PPM frames piped to ffmpeg. Drawing the pixels
// here rather than in an ffmpeg filter graph keeps the curve in the same eight
// lines of arithmetic as the SVG — one function, two renderers, so the still
// and the clip cannot drift apart.

import { spawn } from 'node:child_process'
import { writeFileSync } from 'node:fs'

const W = 640
const H = 360
const X0 = 68, X1 = W - 34, Y0 = 38, Y1 = H - 52

// Where along the span the review happens. Retention returns to 1 there, and
// the second decay is slower: that IS the spacing effect the fixture claims,
// so the picture and the note say the same thing.
const REVIEW = 0.46

/** Retention at u ∈ [0,1] of the plotted span. */
export function retention(u) {
  if (u < REVIEW) return Math.exp(-(u / REVIEW) * 2.5)
  return Math.exp(-((u - REVIEW) / (1 - REVIEW)) * 1.15)
}

const px = (u) => X0 + u * (X1 - X0)
const py = (r) => Y1 - r * (Y1 - Y0)

const PAPER = [247, 244, 236]
const INK = [43, 52, 64]
const AXIS = [124, 136, 150]
const CURVE = [193, 67, 33]
const MARK = [37, 99, 235]
// The finished shape, held under the drawing head from the first frame.
//
// Without it the clip opens on an empty rectangle: a video element shows its
// first frame as a poster while paused, so a clip that starts blank reads as a
// broken embed in the one shot that is meant to prove media renders.
const GHOST = [232, 206, 196]

// ---- the still -------------------------------------------------------------

function polyline(from, to, step = 0.002) {
  const pts = []
  for (let u = from; u <= to + 1e-9; u += step) pts.push(`${px(u).toFixed(1)},${py(retention(u)).toFixed(1)}`)
  return pts.join(' ')
}

const rgb = ([r, g, b]) => `rgb(${r},${g},${b})`

export function curveSvg() {
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" font-family="Nunito, system-ui, sans-serif">
  <rect width="${W}" height="${H}" fill="${rgb(PAPER)}"/>
  <g stroke="${rgb(AXIS)}" stroke-width="1.5" fill="none">
    <line x1="${X0}" y1="${Y0 - 8}" x2="${X0}" y2="${Y1}"/>
    <line x1="${X0}" y1="${Y1}" x2="${X1 + 8}" y2="${Y1}"/>
  </g>
  <g stroke="${rgb(MARK)}" stroke-width="1.5" stroke-dasharray="5 4" opacity="0.85">
    <line x1="${px(REVIEW).toFixed(1)}" y1="${Y0 - 4}" x2="${px(REVIEW).toFixed(1)}" y2="${Y1}"/>
  </g>
  <polyline points="${polyline(0, REVIEW - 0.002)}" fill="none" stroke="${rgb(CURVE)}" stroke-width="3.5" stroke-linecap="round"/>
  <polyline points="${polyline(REVIEW, 1)}" fill="none" stroke="${rgb(CURVE)}" stroke-width="3.5" stroke-linecap="round"/>
  <g fill="${rgb(INK)}" font-size="15">
    <text x="${X0 - 12}" y="${Y0 + 4}" text-anchor="end">1</text>
    <text x="${X0 - 12}" y="${Y1 + 5}" text-anchor="end">0</text>
    <text x="${X0 - 46}" y="${(Y0 + Y1) / 2}" transform="rotate(-90 ${X0 - 46} ${(Y0 + Y1) / 2})" text-anchor="middle">retention</text>
    <text x="${(X0 + X1) / 2}" y="${Y1 + 30}" text-anchor="middle">time since learning</text>
    <text x="${px(REVIEW) + 8}" y="${Y0 + 6}" fill="${rgb(MARK)}" font-size="14" font-weight="700">review</text>
  </g>
</svg>
`
}

// ---- the clip --------------------------------------------------------------

const FPS = 20
const SECONDS = 7
// The curve is drawn over this window; the rest of the clip holds the finished
// figure, so a viewer who arrives late still sees the whole shape.
const DRAW_FROM = 0.5
const DRAW_TO = 5.3

function blank() {
  const b = Buffer.alloc(W * H * 3)
  for (let i = 0; i < b.length; i += 3) {
    b[i] = PAPER[0]; b[i + 1] = PAPER[1]; b[i + 2] = PAPER[2]
  }
  return b
}

function dot(buf, x, y, radius, color) {
  const r2 = radius * radius
  for (let dy = -radius; dy <= radius; dy++) {
    for (let dx = -radius; dx <= radius; dx++) {
      if (dx * dx + dy * dy > r2) continue
      const ix = Math.round(x + dx)
      const iy = Math.round(y + dy)
      if (ix < 0 || iy < 0 || ix >= W || iy >= H) continue
      const o = (iy * W + ix) * 3
      buf[o] = color[0]; buf[o + 1] = color[1]; buf[o + 2] = color[2]
    }
  }
}

function segment(buf, x1, y1, x2, y2, width, color) {
  const steps = Math.ceil(Math.max(Math.abs(x2 - x1), Math.abs(y2 - y1))) + 1
  for (let i = 0; i <= steps; i++) {
    const k = i / steps
    dot(buf, x1 + (x2 - x1) * k, y1 + (y2 - y1) * k, width, color)
  }
}

function axes(buf) {
  segment(buf, X0, Y0 - 8, X0, Y1, 1, AXIS)
  segment(buf, X0, Y1, X1 + 8, Y1, 1, AXIS)
}

// Drawn only once the head has passed it: the marker is the claim that a review
// happened, and showing it before the curve reaches it gives the answer away.
function reviewMarker(buf, progress) {
  if (progress < REVIEW) return
  const x = px(REVIEW)
  for (let y = Y0 - 4; y < Y1; y += 9) segment(buf, x, y, x, Math.min(y + 5, Y1), 1, MARK)
}

function ghost(buf) {
  let prev = null
  for (let u = 0; u <= 1; u += 0.0015) {
    const p = { x: px(u), y: py(retention(u)) }
    if (prev && !(prev.u < REVIEW && u >= REVIEW)) segment(buf, prev.x, prev.y, p.x, p.y, 1, GHOST)
    prev = { ...p, u }
  }
}

function curveTo(buf, progress) {
  let prev = null
  for (let u = 0; u <= progress; u += 0.0015) {
    const p = { x: px(u), y: py(retention(u)) }
    // The review is a discontinuity — joining across it would draw a vertical
    // wall the function does not have.
    if (prev && !(prev.u < REVIEW && u >= REVIEW)) segment(buf, prev.x, prev.y, p.x, p.y, 2, CURVE)
    prev = { ...p, u }
  }
  if (prev) dot(buf, prev.x, prev.y, 5, CURVE)
}

/** Render the clip to `path`. Requires ffmpeg on PATH. */
export function writeCurveVideo(path) {
  return new Promise((resolve, reject) => {
    const ff = spawn('ffmpeg', [
      '-y', '-loglevel', 'error',
      '-f', 'image2pipe', '-vcodec', 'ppm', '-framerate', String(FPS), '-i', '-',
      '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-crf', '23',
      '-movflags', '+faststart', path,
    ])
    ff.on('error', (e) =>
      reject(new Error(`ffmpeg is needed to build the fixture's video: ${e.message}`)))
    ff.on('close', (code) =>
      code === 0 ? resolve(path) : reject(new Error(`ffmpeg exited ${code}`)))

    const header = Buffer.from(`P6\n${W} ${H}\n255\n`)
    let i = 0
    const total = FPS * SECONDS
    const write = () => {
      while (i < total) {
        const t = i / FPS
        const progress = Math.max(0, Math.min(1, (t - DRAW_FROM) / (DRAW_TO - DRAW_FROM)))
        const buf = blank()
        axes(buf)
        ghost(buf)
        reviewMarker(buf, progress)
        curveTo(buf, progress)
        i++
        if (!ff.stdin.write(Buffer.concat([header, buf]))) {
          ff.stdin.once('drain', write)
          return
        }
      }
      ff.stdin.end()
    }
    write()
  })
}

export function writeCurveSvg(path) {
  writeFileSync(path, curveSvg())
  return path
}
