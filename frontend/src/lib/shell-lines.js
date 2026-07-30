// This module is separate so it can be tested by Node without wailsjs dependencies.
// The consumer `shell-run.js` imports `EventsOn` from wailsjs/runtime/runtime,
// which is gitignored and therefore absent on a fresh clone. This module holds
// only the pure line-buffer logic and is testable in isolation.

// Scrollback lives here, in memory, and is never persisted: SaveNotebook writes
// every cell as one JSON blob on a debounce, so keeping megabytes of `docker
// logs` output on the cell would rewrite the whole notebook repeatedly. The
// promoted N4L is the durable record; this buffer is scratch.
export const MAX_LINES = 5000

// appendChunks folds tagged chunks into a line buffer.
//
// A chunk boundary is not a line boundary — the backend reads fixed-size blocks,
// so a line routinely arrives in pieces. The tail line stays "open" until a
// newline closes it, but only continues if the next chunk is the SAME stream;
// otherwise a partial stdout line would swallow the stderr text that follows.
//
// Returns a new array (Svelte reactivity needs the assignment), capped to
// maxLines by dropping the oldest — without which `yes` exhausts the webview.
export function appendChunks(buffer, chunks, maxLines = MAX_LINES) {
  if (!Array.isArray(chunks) || chunks.length === 0) return buffer

  let out = buffer
  let copied = false
  const ensureCopy = () => {
    if (!copied) { out = buffer.slice(); copied = true }
  }

  // Reconstruct which line is open for each stream by scanning the existing buffer.
  // There can be at most one open line per stream (the last one added for that stream).
  const openByStream = {}
  for (let i = out.length - 1; i >= 0; i--) {
    const line = out[i]
    if (!openByStream[line.stream] && line.open) {
      openByStream[line.stream] = line
    }
  }

  for (const chunk of chunks) {
    const text = String(chunk?.text ?? '').replace(/\r/g, '')
    if (!text) continue
    ensureCopy()

    const stream = chunk?.stream === 'stderr' ? 'stderr' : 'stdout'
    const parts = text.split('\n')
    const openLine = openByStream[stream] || null

    // parts[0] continues an open line of the same stream, if there is one.
    if (openLine && openLine.open) {
      openLine.text += parts[0]
    } else {
      // Start a new line (even if parts[0] is empty, which represents a blank
      // line when chunk starts with a newline). This ensures the byte stream
      // is faithfully reconstructed regardless of chunk boundaries.
      const newLine = { stream, text: parts[0], open: true }
      out.push(newLine)
      openByStream[stream] = newLine
    }

    for (let i = 1; i < parts.length; i++) {
      // The line before each newline is now closed.
      if (openByStream[stream]) {
        openByStream[stream].open = false
      }
      const isLast = i === parts.length - 1
      // A trailing newline leaves nothing open; anything after it starts a new
      // open line.
      if (!isLast || parts[i] !== '') {
        const newLine = { stream, text: parts[i], open: true }
        out.push(newLine)
        openByStream[stream] = newLine
      } else {
        openByStream[stream] = null
      }
    }
  }

  if (copied && out.length > maxLines) out = out.slice(out.length - maxLines)
  return out
}
