// Shapes an LLM conversation into a markdown study document.
//
// Questions become headings and answers become bodies, rather than a
// You:/Assistant: transcript: the result is a source you will read
// incrementally and extract from, and your half-formed questions should not be
// part of the text you study.
//
// Kept out of the component because this repo has no Svelte component test
// harness — logic left in a .svelte file has no tests at all.

/**
 * @param {string} title
 * @param {Array<{role: string, content: string}>} messages
 * @param {string[]} models  model ids used during the chat, in first-use order
 * @param {Date} date
 * @returns {string}
 */
export function chatToMarkdown(title, messages, models, date = new Date()) {
  const list = Array.isArray(messages) ? messages : []
  const out = ['# ' + String(title ?? '').trim(), '']

  for (let i = 0; i < list.length; i++) {
    if (list[i]?.role !== 'user') continue
    const answer = list[i + 1]
    // Drop a question whose answer has not arrived rather than saving an empty
    // section — this is a study document, not a conversation in progress.
    if (answer?.role !== 'assistant') continue
    out.push('## ' + oneLine(list[i].content), '', String(answer.content ?? '').trim(), '')
  }

  const used = [...new Set((models ?? []).filter(Boolean))]
  if (used.length) {
    // Provenance, for the same reason the medium is "llm": a reader months from
    // now should be able to see this was written by a model, and by which.
    // toISOString is UTC, so a save near local midnight can name the next day —
    // acceptable for a provenance line, and it keeps the output deterministic.
    out.push('---', '*Generated with ' + used.join(', ') + ' — ' + date.toISOString().slice(0, 10) + '*', '')
  }
  return out.join('\n')
}

/** First line of the first question, capped — a starting point to edit. */
export function defaultTitle(messages) {
  const first = (Array.isArray(messages) ? messages : []).find((m) => m?.role === 'user')
  if (!first) return ''
  // The first LINE, not the whole question collapsed. A multi-line question's
  // later lines are supporting detail; the title should be the thing you asked.
  // (chatToMarkdown's heading differs deliberately — there the whole question
  // becomes the heading, folded onto one line.)
  return oneLine(String(first.content ?? '').split('\n')[0]).slice(0, 60)
}

// A heading cannot contain a newline, and stray runs of whitespace read badly
// in a table of contents.
function oneLine(s) {
  return String(s ?? '').replace(/\s+/g, ' ').trim()
}
