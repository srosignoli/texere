// Resolves a caret line to the node it sits in, from the span map FileNodeSpans
// returns.
//
// Deliberately dependency-free so Node's test runner can load it: this repo has
// no Svelte component harness, so anything left inside a .svelte file is
// untested, and this is where all the edge cases live.

// nodeIdAtLine returns the id of the node containing `line`, or null when the
// line is not inside any id-bearing node — a blank line, a chapter title, a
// context marker, a plain note with no id, or a position past the end of the
// file after an edit shortened it.
//
// Bounds are 1-indexed and inclusive at both ends, matching content.NodeSpan.
// Spans do not overlap, so the first match is the answer.
export function nodeIdAtLine(spans, line) {
  if (!Array.isArray(spans)) return null
  for (const s of spans) {
    if (line >= s.fromLine && line <= s.toLine) return s.id
  }
  return null
}
