import { getStroke } from 'perfect-freehand'

const STROKE_OPTS = { thinning: 0.6, smoothing: 0.5, streamline: 0.5, simulatePressure: true }

// Build a filled Path2D from a perfect-freehand outline polygon.
function pathFromOutline(outline) {
  const p = new Path2D()
  if (!outline.length) return p
  p.moveTo(outline[0][0], outline[0][1])
  for (let i = 1; i < outline.length; i++) p.lineTo(outline[i][0], outline[i][1])
  p.closePath()
  return p
}

// renderStrokes clears the canvas and fills each stroke. width/height are CSS
// pixels; dpr scales the backing store so strokes are crisp on HiDPI displays.
export function renderStrokes(ctx, strokes, { width, height, dpr = 1 }) {
  const canvas = ctx.canvas
  const bw = Math.max(1, Math.round(width * dpr))
  const bh = Math.max(1, Math.round(height * dpr))
  if (canvas.width !== bw) canvas.width = bw
  if (canvas.height !== bh) canvas.height = bh
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)
  for (const s of strokes ?? []) {
    if (!s || !s.points || !s.points.length) continue
    const outline = getStroke(s.points, { size: s.size ?? 3, ...STROKE_OPTS })
    const path = pathFromOutline(outline)
    ctx.save()
    if (s.tool === 'highlighter') {
      ctx.globalAlpha = 0.35
      ctx.globalCompositeOperation = 'multiply'
    }
    ctx.fillStyle = s.color ?? '#e8eaf0'
    ctx.fill(path)
    ctx.restore()
  }
}
