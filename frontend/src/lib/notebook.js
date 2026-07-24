import ContextCell from '../components/notebook/ContextCell.svelte'
import N4LDraftCell from '../components/notebook/N4LDraftCell.svelte'
import LLMCell from '../components/notebook/LLMCell.svelte'
import GraphQueryCell from '../components/notebook/GraphQueryCell.svelte'
import ComputeCell from '../components/notebook/ComputeCell.svelte'
import SketchCell from '../components/notebook/SketchCell.svelte'
import ImageCell from '../components/notebook/ImageCell.svelte'

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

// type -> Svelte component. Later slices add: image.
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
  llm: LLMCell,
  graph: GraphQueryCell,
  compute: ComputeCell,
  sketch: SketchCell,
  image: ImageCell,
}
