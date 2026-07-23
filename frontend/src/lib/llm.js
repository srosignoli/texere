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

// buildSystem composes the system prompt: mode framing + a context block built
// from the pinned context cell and any attached cells' text.
export function buildSystem(mode, context, attachedCells) {
  const parts = [mode === 'tutor' ? TUTOR_SYSTEM : ASK_SYSTEM]
  const ctx = []
  if (context?.extractText) ctx.push(`# Extract\n${context.extractText}`)
  if (context?.tags?.length) ctx.push(`# Tags\n${context.tags.join(', ')}`)
  if (context?.orbit?.length) {
    ctx.push('# Neighbours\n' + context.orbit.map((o) => `- ${o.arrowLong} ${o.neighborText}`).join('\n'))
  }
  if (context?.source?.passage) ctx.push(`# Source passage\n${context.source.passage}`)
  for (const c of attachedCells ?? []) {
    if (c.type === 'n4l' && c.text) ctx.push(`# Attached N4L draft\n${c.text}`)
    else if (c.type === 'llm' && c.messages?.length) {
      ctx.push('# Attached conversation\n' + c.messages.map((m) => `${m.role}: ${m.content}`).join('\n'))
    }
    else if (c.type === 'graph' && c.resultText) ctx.push(`# Attached graph result\n${c.resultText}`)
    else if (c.type === 'compute' && c.resultText) ctx.push(`# Attached compute output\n${c.resultText}`)
  }
  if (ctx.length) parts.push('--- Context ---\n' + ctx.join('\n\n'))
  return parts.join('\n\n')
}

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
