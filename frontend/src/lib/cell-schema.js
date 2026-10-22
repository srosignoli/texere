// The schema of a Notebook's cells: the factory for each cell type, and the
// rules that change a cell's fields. Go stores cells as opaque JSON, so this
// file is the schema.
//
// Deliberately dependency-free, like cell-label.js: notebook.js imports the
// cell components for cellRegistry, which Node's test runner cannot parse.

let seq = 0
export function newDraftCell(text = '') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'n4l', text }
}

export function newLLMCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'llm',
    skill: 'thinking-partner', skillName: 'Thinking partner',
    profileId: null, messages: [], attachments: [],
  }
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
    durationMs: 0,     // audio and video; set by the pick
    crop: null, naturalW: 0, naturalH: 0,
    query: '', subject: '', lang: '',
    provider: '', sourcePage: '', license: '',
  }
}

// Shell cell: runs one command in a chosen OS shell. `shell`/`shellLabel` are
// the dropdown's id and display text; scrollback is deliberately absent, since
// it is never persisted. See the shell-cell design doc.
export function newShellCell() {
  seq += 1
  return {
    id: `cell-${Date.now()}-${seq}`, type: 'shell',
    shell: '', shellLabel: '', cwd: '', command: '', subject: '',
  }
}

// pickMediaAsset puts a newly picked asset on a media cell: the search or
// import `result` that found it and the stored `ref` ({ asset, mime }). It
// changes the cell in place, as cells are mutated in place throughout the
// notebook.
export function pickMediaAsset(cell, result, ref) {
  cell.asset = ref.asset
  cell.mime = ref.mime
  cell.kind = result.kind || 'image'
  cell.provider = result.provider ?? cell.provider
  cell.sourcePage = result.sourcePage ?? ''
  cell.license = result.license ?? ''
  // The thing you searched for is the thing the fact will be about.
  if (!cell.subject) cell.subject = (cell.query ?? '').trim()
  // A crop is fractions of the OLD frame; carrying it onto a differently
  // framed picture silently mis-crops it, so drop it with the asset.
  cell.crop = null
  cell.naturalW = 0
  cell.naturalH = 0
  cell.embedURL = result.embedURL ?? ''
  cell.playerKind = result.playerKind ?? ''
  cell.title = result.title ?? ''
  cell.startSec = 0
  cell.durationMs = result.durationMs ?? 0
}

// ATTACHABLE_TYPES are the cell types an LLM cell can attach as context.
export const ATTACHABLE_TYPES = ['n4l', 'llm', 'graph', 'compute', 'sketch', 'image']
