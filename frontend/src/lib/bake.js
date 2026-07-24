// bake.js — render a sketch or image cell to a downscaled base64 image for
// multimodal LLM turns. Returns { mediaType, data } with data = base64 (no prefix).
import { renderStrokes } from './sketch-render.js'

const MAX_EDGE = 1568 // Anthropic's recommended max long edge
const SKETCH_BG = '#0b1220' // dark bg preserves contrast for light strokes

function loadImage(src) {
  return new Promise((resolve, reject) => {
    const im = new Image()
    im.onload = () => resolve(im)
    im.onerror = () => reject(new Error('image not found: ' + src))
    im.src = src
  })
}

export async function bakeSketch(cell) {
  const strokes = cell.strokes ?? []
  if (!strokes.length) throw new Error('Sketch is empty — nothing to send.')
  const lw = cell.logicalW || 800
  const lh = cell.logicalH || 600
  const s = Math.min(1, MAX_EDGE / Math.max(lw, lh))
  const w = Math.max(1, Math.round(lw * s))
  const h = Math.max(1, Math.round(lh * s))
  const strokeCanvas = document.createElement('canvas')
  renderStrokes(strokeCanvas.getContext('2d'), strokes, { width: w, height: h, dpr: 1, logicalW: lw, logicalH: lh })
  const out = document.createElement('canvas')
  out.width = w
  out.height = h
  const ctx = out.getContext('2d')
  ctx.fillStyle = SKETCH_BG
  ctx.fillRect(0, 0, w, h)
  ctx.drawImage(strokeCanvas, 0, 0)
  return { mediaType: 'image/png', data: out.toDataURL('image/png').split(',')[1] }
}

// Rotate the context so drawing the natural image at (0,0) fills the rotated
// frame RW×RH (r ∈ {0,90,180,270} clockwise). Mirrors image-view.js geometry.
function applyRotation(ctx, r, RW, RH) {
  if (r === 90) { ctx.translate(RW, 0); ctx.rotate(Math.PI / 2) }
  else if (r === 180) { ctx.translate(RW, RH); ctx.rotate(Math.PI) }
  else if (r === 270) { ctx.translate(0, RH); ctx.rotate(3 * Math.PI / 2) }
}

export async function bakeImage(cell) {
  const img = await loadImage(`/media/${cell.asset}`)
  const W = img.naturalWidth
  const H = img.naturalHeight
  const r = (((Math.round((cell.rotation || 0) / 90) * 90) % 360) + 360) % 360
  const swap = r === 90 || r === 270
  const RW = swap ? H : W
  const RH = swap ? W : H
  const c = cell.crop && cell.crop.w > 0 && cell.crop.h > 0 ? cell.crop : { x: 0, y: 0, w: 1, h: 1 }
  const cw = c.w * RW
  const ch = c.h * RH
  const s = Math.min(1, MAX_EDGE / Math.max(cw, ch))
  const outW = Math.max(1, Math.round(cw * s))
  const outH = Math.max(1, Math.round(ch * s))
  const canvas = document.createElement('canvas')
  canvas.width = outW
  canvas.height = outH
  const ctx = canvas.getContext('2d')
  ctx.scale(s, s)               // output px → rotated-frame px
  ctx.translate(-c.x * RW, -c.y * RH) // pan so the crop origin maps to (0,0)
  applyRotation(ctx, r, RW, RH) // rotate the frame, then draw the natural image at 0,0
  ctx.drawImage(img, 0, 0)
  return { mediaType: 'image/jpeg', data: canvas.toDataURL('image/jpeg', 0.85).split(',')[1] }
}
