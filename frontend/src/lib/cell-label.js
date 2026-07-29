// Header text for a stacked notebook cell. Deliberately its own dependency-free
// module: notebook.js pulls in nine .svelte components for cellRegistry, which
// Node's test runner cannot parse, so nothing there is unit-testable.

// Display name per stacked cell type, matching the button that creates it — the
// same word for creating and for identifying. `context` is deliberately absent:
// ContextCell is rendered above the stack rather than inside it, so it never
// needs a header; the fallback in cellLabel covers it if one ever appears.
const CELL_NAMES = {
  n4l: 'N4L draft',
  llm: 'LLM',
  graph: 'Graph',
  compute: 'Compute',
  sketch: 'Sketch',
  image: 'Scan',
  audio: 'Audio',
  media: 'Media',
}

// mediaDetail names what a media cell actually holds. Three stacked cells all
// reading "Media" — a picture, a pronunciation and a video — is precisely the
// confusion these headers exist to remove. A pronunciation is called that
// rather than "audio" because it is the word a learner is scanning for.
function mediaDetail(cell) {
  const kind = cell.kind || ''
  if (kind === 'audio' && cell.provider === 'pronunciation') return 'pronunciation'
  return kind
}

// cellLabel is the text of a stacked cell's header: the type's name, plus a
// detail when that type has meaningful variants. Kept pure and outside the
// component so it is unit-testable without mounting Svelte.
//
// An unknown type falls back to the raw type string, and a type-less cell to
// "Cell", so a header is never blank — a blank strip reads as a rendering bug.
// A missing detail appends nothing rather than a dangling separator.
export function cellLabel(cell) {
  const type = cell?.type ?? ''
  const name = CELL_NAMES[type] || type || 'Cell'
  let detail = ''
  if (type === 'llm') detail = cell.mode || ''
  else if (type === 'compute') detail = cell.language || ''
  else if (type === 'media') detail = mediaDetail(cell)
  return detail ? `${name} · ${detail}` : name
}
