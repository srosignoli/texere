// Composes the system prompt sent with every LLM turn: the selected skill's
// body, followed by a context block assembled from the notebook.
//
// Deliberately dependency-free. Its sibling llm.js imports the Wails runtime
// from ../../wailsjs/runtime/runtime, which is gitignored and therefore absent
// on a fresh clone — so anything living there cannot be loaded by Node's test
// runner. buildSystem composes the entire prompt and had no test at all for
// exactly that reason. Keep this file importing nothing.

// The skill a cell falls back to when it names none.
export const DEFAULT_SKILL_ID = 'thinking-partner'

// Notebooks saved before skills existed carry `mode: 'ask' | 'tutor'`.
const LEGACY_MODE_SKILLS = {
  ask: 'thinking-partner',
  tutor: 'socratic-tutor',
}

// skillIdForCell resolves which skill a cell wants.
//
// Migration is deliberately lazy: the legacy `mode` is mapped here, at read
// time, rather than by rewriting saved notebooks. Cells persist as one opaque
// JSON blob, so the stale field costs nothing and no file is ever touched to
// convert it — a rewrite pass over a user's notes is a risk with no upside.
export function skillIdForCell(cell) {
  if (cell?.skill) return cell.skill
  return LEGACY_MODE_SKILLS[cell?.mode] ?? DEFAULT_SKILL_ID
}

// buildSystem prepends the skill body to a context block built from the pinned
// context cell and any attached cells' text.
//
// The first argument is the system text itself, not a mode: choosing it belongs
// to the caller, which knows the selected skill. This function only composes.
export function buildSystem(systemText, context, attachedCells) {
  const parts = [systemText]
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
    else if (c.type === 'compute' && (c.code || c.resultText)) {
      let s = '# Attached compute cell'
      if (c.code) s += `\n\`\`\`js\n${c.code}\n\`\`\``
      if (c.resultText) s += `\n\nOutput:\n${c.resultText}`
      ctx.push(s)
    }
  }
  if (ctx.length) parts.push('--- Context ---\n' + ctx.join('\n\n'))
  return parts.join('\n\n')
}
