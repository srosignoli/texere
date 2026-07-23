import ContextCell from '../components/notebook/ContextCell.svelte'
import N4LDraftCell from '../components/notebook/N4LDraftCell.svelte'
import LLMCell from '../components/notebook/LLMCell.svelte'
import GraphQueryCell from '../components/notebook/GraphQueryCell.svelte'

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

// type -> Svelte component. Later slices add: compute, sketch, image.
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
  llm: LLMCell,
  graph: GraphQueryCell,
}
