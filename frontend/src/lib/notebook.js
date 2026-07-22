import ContextCell from '../components/notebook/ContextCell.svelte'
import N4LDraftCell from '../components/notebook/N4LDraftCell.svelte'

let seq = 0
export function newDraftCell(text = '') {
  seq += 1
  return { id: `cell-${Date.now()}-${seq}`, type: 'n4l', text }
}

// type -> Svelte component. Later slices add: llm, graph, compute, sketch, image.
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
}
