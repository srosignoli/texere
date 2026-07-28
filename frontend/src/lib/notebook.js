// Svelte component imports are dynamically loaded to avoid breaking Node.js testing.
// When this module is imported in a browser (via Vite), dynamic imports work fine.
// When imported in Node.js for testing, these imports fail gracefully, which is okay
// because cellRegistry is not used by the cellLabel function.
let _cellRegistry

async function loadCellRegistry() {
  if (_cellRegistry) return _cellRegistry
  try {
    const [
      ContextCell,
      N4LDraftCell,
      LLMCell,
      GraphQueryCell,
      ComputeCell,
      SketchCell,
      ImageCell,
      AudioCell,
      MediaCell,
    ] = await Promise.all([
      import('../components/notebook/ContextCell.svelte'),
      import('../components/notebook/N4LDraftCell.svelte'),
      import('../components/notebook/LLMCell.svelte'),
      import('../components/notebook/GraphQueryCell.svelte'),
      import('../components/notebook/ComputeCell.svelte'),
      import('../components/notebook/SketchCell.svelte'),
      import('../components/notebook/ImageCell.svelte'),
      import('../components/notebook/AudioCell.svelte'),
      import('../components/notebook/MediaCell.svelte'),
    ])
    _cellRegistry = {
      context: ContextCell.default,
      n4l: N4LDraftCell.default,
      llm: LLMCell.default,
      graph: GraphQueryCell.default,
      compute: ComputeCell.default,
      sketch: SketchCell.default,
      image: ImageCell.default,
      audio: AudioCell.default,
      media: MediaCell.default,
    }
  } catch (err) {
    // In Node.js testing, Svelte imports fail. Create a minimal stub so the
    // module can still be imported.
    _cellRegistry = {}
  }
  return _cellRegistry
}

// For Vite/browser environments, eagerly initialize so Svelte components
// can access cellRegistry synchronously. For Node.js, this creates an
// empty object that won't be fully populated.
export const cellRegistry = {}
loadCellRegistry().then(reg => Object.assign(cellRegistry, reg)).catch(() => {})

let seq = 0
export function newDraftCell(text = '') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'n4l', text }
}

export function newLLMCell(mode = 'ask') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'llm', mode, profileId: null, messages: [], attachments: [] }
}

export function newGraphCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'graph', query: '', resultText: '' }
}

export function newComputeCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'compute', language: 'js', code: '', resultText: '' }
}

export function newSketchCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'sketch', height: 240, strokes: [] }
}

export function newImageCell(asset, mime) {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'image', asset, mime, naturalW: 0, naturalH: 0, rotation: 0, crop: null }
}

export function newAudioCell() {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'audio', asset: '', mime: '', durationMs: 0 }
}

// Media cell: a study asset (picture now; pronunciation and video later) found
// by web search, pasted URL, or file import. `kind` discriminates the render;
// `embedURL` is reserved for video, which is referenced rather than stored, so
// adding it later needs no migration of saved notebooks.
export function newMediaCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'media',
    kind: 'image',
    asset: '', mime: '', embedURL: '',
    playerKind: '',    // 'file' | 'iframe'; video only
    title: '',         // video only; drives the promoted (note) line
    startSec: 0,       // video only; the marked moment
    crop: null, naturalW: 0, naturalH: 0,
    query: '', subject: '', lang: '',
    provider: '', sourcePage: '', license: '',
  }
}

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

