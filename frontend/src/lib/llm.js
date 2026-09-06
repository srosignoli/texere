import { StartLLMTurn, CancelLLMTurn } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { createDeltaOrderer } from './delta-order.js'

export const TRANSCRIBE_SYSTEM =
  'You transcribe images of handwritten or printed notes into clean text. ' +
  'Transcribe all text faithfully, preserving structure (headings, lists, indentation) as Markdown. ' +
  'If the image contains a diagram, flowchart, chart, mind-map, or graph, reconstruct it as a fenced ' +
  '```mermaid code block that best represents its structure, in addition to any surrounding text. ' +
  'Output only the transcription — no preamble or commentary.'

// streamTurn starts a streamed turn and wires the delta/done/error events,
// filtered to this turn's streamId. Returns a cancel() that stops the stream
// and unsubscribes.
//
// onDone receives the COMPLETE reply, which llm:done carries. Callers should
// prefer it over whatever they accumulated from onDelta: the deltas are a live
// preview, and the preview is the only part that a delivery race can disturb.
//
// Deltas arrive with a sequence number because they can arrive out of order —
// see the note on llmEmitInterval in llm_methods.go for why. They are buffered
// here and released in order, so the preview reads correctly too.
export function streamTurn({ profileId, system, messages, onDelta, onDone, onError }) {
  let offDelta = () => {}
  let offDone = () => {}
  let offError = () => {}
  let sid = null
  let cancelled = false

  // Buffers deltas that overtook their predecessor. The rule itself lives in
  // delta-order.js, which imports nothing, so it can be tested — this file
  // cannot be, since the Wails runtime it imports is absent on a fresh clone.
  const ordered = createDeltaOrderer((text) => onDelta(text))

  StartLLMTurn({ profileId: profileId || '', system, messages })
    .then((streamId) => {
      sid = streamId
      if (cancelled) { CancelLLMTurn(sid); return }
      offDelta = EventsOn('llm:delta', (p) => {
        if (p?.streamId !== sid) return
        ordered.push(p.seq, p.text)
      })
      offDone = EventsOn('llm:done', (p) => {
        if (p?.streamId !== sid) return
        // The preview may still be holding a delta whose predecessor never
        // arrived; release it so a cancelled-looking gap does not linger. The
        // committed text comes from the event either way.
        ordered.flush()
        cleanup()
        onDone(typeof p.text === 'string' ? p.text : '')
      })
      offError = EventsOn('llm:error', (p) => { if (p?.streamId === sid) { cleanup(); onError(p.message) } })
    })
    .catch((e) => onError(String(e?.message ?? e)))

  function cleanup() { offDelta(); offDone(); offError() }
  function cancel() { cancelled = true; if (sid) CancelLLMTurn(sid); cleanup() }
  return { cancel }
}
