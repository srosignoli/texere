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

// fitTransform computes a uniform scale-to-fit (letterbox) mapping from a
// sketch's fixed logical space (logicalW × logicalH) into a container of
// containerW × containerH CSS px. Returns { scale, ox, oy } so a logical point
// (lx, ly) maps to CSS (ox + lx*scale, oy + ly*scale). When logical dims are
// unset/invalid, returns identity so legacy absolute-pixel sketches render
// unchanged.
export function fitTransform(containerW, containerH, logicalW, logicalH) {
  if (!logicalW || !logicalH || logicalW <= 0 || logicalH <= 0) return { scale: 1, ox: 0, oy: 0 }
  const scale = Math.min(containerW / logicalW, containerH / logicalH)
  const ox = (containerW - logicalW * scale) / 2
  const oy = (containerH - logicalH * scale) / 2
  return { scale, ox, oy }
}

// renderStrokes clears the canvas and fills each stroke. width/height are CSS
// pixels; dpr scales the backing store so strokes are crisp on HiDPI displays.
// logicalW/logicalH define the sketch's logical space; the fit transform (scale
// + letterbox offset) is baked into the canvas transform so logical stroke
// coordinates AND sizes map into the container. Unset logical dims → identity
// (legacy absolute-pixel strokes render unchanged).
export function renderStrokes(ctx, strokes, { width, height, dpr = 1, logicalW = 0, logicalH = 0 }) {
  const canvas = ctx.canvas
  const bw = Math.max(1, Math.round(width * dpr))
  const bh = Math.max(1, Math.round(height * dpr))
  if (canvas.width !== bw) canvas.width = bw
  if (canvas.height !== bh) canvas.height = bh
  // Clear in device space, then apply the fit transform baked with dpr.
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  ctx.clearRect(0, 0, bw, bh)
  const { scale, ox, oy } = fitTransform(width, height, logicalW, logicalH)
  ctx.setTransform(dpr * scale, 0, 0, dpr * scale, dpr * ox, dpr * oy)
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
