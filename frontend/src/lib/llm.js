import { StartLLMTurn, CancelLLMTurn } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export const TRANSCRIBE_SYSTEM =
  'You transcribe images of handwritten or printed notes into clean text. ' +
  'Transcribe all text faithfully, preserving structure (headings, lists, indentation) as Markdown. ' +
  'If the image contains a diagram, flowchart, chart, mind-map, or graph, reconstruct it as a fenced ' +
  '```mermaid code block that best represents its structure, in addition to any surrounding text. ' +
  'Output only the transcription — no preamble or commentary.'

// streamTurn starts a streamed turn and wires the delta/done/error events,
// filtered to this turn's streamId. Returns a cancel() that stops the stream
// and unsubscribes.
export function streamTurn({ profileId, system, messages, onDelta, onDone, onError }) {
  let offDelta = () => {}
  let offDone = () => {}
  let offError = () => {}
  let sid = null
  let cancelled = false

  StartLLMTurn({ profileId: profileId || '', system, messages })
    .then((streamId) => {
      sid = streamId
      if (cancelled) { CancelLLMTurn(sid); return }
      offDelta = EventsOn('llm:delta', (p) => { if (p?.streamId === sid) onDelta(p.text) })
      offDone = EventsOn('llm:done', (p) => { if (p?.streamId === sid) { cleanup(); onDone() } })
      offError = EventsOn('llm:error', (p) => { if (p?.streamId === sid) { cleanup(); onError(p.message) } })
    })
    .catch((e) => onError(String(e?.message ?? e)))

  function cleanup() { offDelta(); offDone(); offError() }
  function cancel() { cancelled = true; if (sid) CancelLLMTurn(sid); cleanup() }
  return { cancel }
}
