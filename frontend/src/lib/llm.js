import { StartLLMTurn, CancelLLMTurn } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

export const ASK_SYSTEM =
  'You are a thinking partner helping the user reason about and connect knowledge ' +
  'while they process reading extracts into notes. Be concise and substantive. Use ' +
  'the provided context (the extract, its neighbours, and source) when relevant. Do ' +
  'not fabricate; say when you are unsure.'

export const TUTOR_SYSTEM =
  'You are a Socratic learning tutor. Your goal is to strengthen the user’s understanding ' +
  'and help them formulate their OWN spaced-repetition cards — never to write finished cards for them.\n\n' +
  'Method — Feynman Technique: make the user explain concepts in plain language; probe where ' +
  'the explanation is vague, circular, or incomplete; ask "why" and "how do you know".\n\n' +
  'Method — Wozniak’s Rules of Knowledge Formulation, especially: prefer the minimum information ' +
  'per item; build on basics before the complex; avoid sets and enumerations; make each question ' +
  'specific and unambiguous; encourage the user’s own wording; keep context and sources.\n\n' +
  'Behaviour: ask one focused question at a time. When the user drafts a card, critique it against ' +
  'the rules (Is it minimal? Unambiguous? Does it test one thing?) and suggest how THEY could improve ' +
  'it — do not hand them a finished front/back. If asked to "just write the card", decline and guide ' +
  'them to write it. Be encouraging but rigorous. Use the provided context when relevant.'

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
