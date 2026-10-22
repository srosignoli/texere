import ContextCell from '../components/notebook/ContextCell.svelte'
import N4LDraftCell from '../components/notebook/N4LDraftCell.svelte'
import LLMCell from '../components/notebook/LLMCell.svelte'
import GraphQueryCell from '../components/notebook/GraphQueryCell.svelte'
import ComputeCell from '../components/notebook/ComputeCell.svelte'
import SketchCell from '../components/notebook/SketchCell.svelte'
import ImageCell from '../components/notebook/ImageCell.svelte'
import AudioCell from '../components/notebook/AudioCell.svelte'
import MediaCell from '../components/notebook/MediaCell.svelte'
import ShellCell from '../components/notebook/ShellCell.svelte'

// type -> Svelte component. The cells themselves are defined in cell-schema.js.
export const cellRegistry = {
  context: ContextCell,
  n4l: N4LDraftCell,
  llm: LLMCell,
  graph: GraphQueryCell,
  compute: ComputeCell,
  sketch: SketchCell,
  image: ImageCell,
  audio: AudioCell,
  media: MediaCell,
  shell: ShellCell,
}
