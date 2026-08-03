// Header text for a stacked notebook cell. Deliberately its own dependency-free
// module: notebook.js pulls in nine .svelte components for cellRegistry, which
// Node's test runner cannot parse, so nothing there is unit-testable.
//
// llm-prompt.js is imported for skillIdForCell: it is documented there as
// dependency-free too (its sibling llm.js is the one that pulls in the
// gitignored Wails runtime), so this stays safe for Node's test runner.
import { skillIdForCell } from './llm-prompt.js'

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
  shell: 'Shell',
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
  // skillName is persisted on the cell so this stays a pure function. When it
  // is absent — a legacy cell that only carries `mode`, or one with neither —
  // fall back to skillIdForCell, the SAME resolution LLMCell.svelte's picker
  // uses (see llm-prompt.js). Echoing the raw `mode` string here instead (the
  // previous behaviour) let the header read "LLM · tutor" while the picker
  // beside it, resolving mode through the real skill list, showed "Socratic
  // tutor" — forever, on the same cell. Showing the resolved id is less
  // pretty than the picker's friendly name, but a friendly name requires the
  // loaded skills list, which is fetched asynchronously and would break this
  // function's purity/synchrony (and its plain `node --test` runnability).
  // The id can never contradict the picker, because it IS the picker's key.
  if (type === 'llm') detail = cell.skillName || skillIdForCell(cell)
  else if (type === 'compute') detail = cell.language || ''
  else if (type === 'media') detail = mediaDetail(cell)
  else if (type === 'shell') detail = cell.shellLabel || ''
  return detail ? `${name} · ${detail}` : name
}
