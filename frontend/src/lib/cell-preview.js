// One line of content-derived text for a folded notebook cell. Deliberately its
// own dependency-free module: notebook.js statically imports nine .svelte
// components for cellRegistry, which Node's test runner cannot parse, so
// nothing reachable from there is unit-testable.
//
// Separate from cell-label.js on purpose. cellLabel reads type/mode/kind and
// always returns something, because a blank header reads as a rendering bug.
// cellPreview reads eight different content fields and is expected to return
// '' — the caller then renders no preview element at all, so a cell with
// nothing in it folds to its header alone rather than to a placeholder.

// Caps what reaches the DOM. This is not the visible truncation: CSS does that,
// at the pane's real width, which is the only place the width is known. The cap
// stops a 40 KB draft's first line being written into the DOM for a cell the
// user folded specifically to stop looking at.
const MAX = 120

function clip(s) {
  const t = String(s ?? '').trim()
  return t.length > MAX ? t.slice(0, MAX) : t
}

// The first line with something on it — a draft that opens with blank lines
// still has content worth previewing.
function firstLine(text) {
  for (const line of String(text ?? '').split('\n')) {
    const t = line.trim()
    if (t) return clip(t)
  }
  return ''
}

// m:ss. Exported because AudioCell renders the same cell's duration a few
// pixels below this preview; two private copies could drift into showing two
// different numbers for one recording. Non-finite and negative input floor to
// zero rather than rendering "NaN:NaN".
export function formatDuration(ms) {
  const n = Number(ms)
  const s = Math.floor((Number.isFinite(n) && n > 0 ? n : 0) / 1000)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// The count says how much conversation is hidden; the opening question says
// what it was about. Selected by role rather than by position so a future
// system message cannot become the preview.
function llmPreview(cell) {
  const msgs = Array.isArray(cell.messages) ? cell.messages : []
  const head = clip(msgs.find((m) => m?.role === 'user')?.content)
  if (!head) return ''
  return `"${head}" · ${msgs.length} ${msgs.length === 1 ? 'msg' : 'msgs'}`
}

function sketchPreview(cell) {
  const n = Array.isArray(cell.strokes) ? cell.strokes.length : 0
  return n ? `${n} ${n === 1 ? 'stroke' : 'strokes'}` : ''
}

// Assets are content-hashed — internal/media/import.go stores sha256hex+ext and
// discards the original filename — so geometry is the only identifying thing an
// image cell persists. Zero until the <img> has loaded once.
function imagePreview(cell) {
  const w = Number(cell.naturalW) || 0
  const h = Number(cell.naturalH) || 0
  return w && h ? `${w}×${h}` : ''
}

export function cellPreview(cell) {
  if (!cell) return ''
  switch (cell.type) {
    case 'n4l': return firstLine(cell.text)
    case 'llm': return llmPreview(cell)
    case 'graph': return clip(cell.query)
    case 'compute': return firstLine(cell.code)
    case 'sketch': return sketchPreview(cell)
    case 'image': return imagePreview(cell)
    case 'audio': return cell.durationMs ? formatDuration(cell.durationMs) : ''
    case 'media': return clip(cell.title || cell.subject || cell.query)
    default: return ''
  }
}
