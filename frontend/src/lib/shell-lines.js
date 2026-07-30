// This module is separate so it can be tested by Node without wailsjs dependencies.
// The consumer `shell-run.js` imports `EventsOn` from wailsjs/runtime/runtime,
// which is gitignored and therefore absent on a fresh clone. This module holds
// only the pure line-buffer logic and is testable in isolation.

// Scrollback lives here, in memory, and is never persisted: SaveNotebook writes
// every cell as one JSON blob on a debounce, so keeping megabytes of `docker
// logs` output on the cell would rewrite the whole notebook repeatedly. The
// promoted N4L is the durable record; this buffer is scratch.
export const MAX_LINES = 5000

// The stream name carried by the truncation marker. It cannot collide with real
// output: appendChunks coerces every incoming chunk's stream to 'stdout' or
// 'stderr', so no command can print a line that ends up tagged this way. The
// marker is a separate stream rather than a stdout line with special text
// because ShellCell.svelte renders it as its own element, OUTSIDE the `.sh-line`
// class that the selection reads — which is what keeps a truncation notice from
// ever becoming an `(out)` fact.
export const NOTICE_STREAM = 'notice'

// truncationNotice is the marker prepended when lines are dropped. It carries
// the running total so successive truncations accumulate rather than each
// claiming only its own batch.
function truncationNotice(dropped, maxLines) {
  return {
    stream: NOTICE_STREAM,
    text: `⋯ ${dropped} earlier line${dropped === 1 ? '' : 's'} dropped — showing the last ${maxLines}`,
    open: false,
    dropped,
  }
}

// appendChunks folds tagged chunks into a line buffer.
//
// A chunk boundary is not a line boundary — the backend reads fixed-size blocks,
// so a line routinely arrives in pieces. The tail line stays "open" until a
// newline closes it, but only continues if the next chunk is the SAME stream;
// otherwise a partial stdout line would swallow the stderr text that follows.
//
// Returns a new array (Svelte reactivity needs the assignment), capped to
// maxLines by dropping the oldest — without which `yes` exhausts the webview.
//
// Ordering: a reunited line stays at its first fragment's array position.
// For example, {stdout,'AAA'} then {stderr,'BBB'} then {stdout,'CCC\n'} yields
// ["stdout:AAACCC", "stderr:BBB"] — the stdout line sorts before the stderr that
// arrived between its halves. This is self-consistent and acceptable: the design
// accepts cross-stream interleaving as best-effort, and ordering by first arrival
// of each line is clearer than trying to sort reunited lines by their completion time.
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

  // Cap the buffer, and say so. Silently dropping the front leaves a reader
  // unable to tell a short command from a truncated one — the design calls for
  // a marker, so one is emitted.
  //
  // The marker does NOT count against maxLines: the cap exists to bound memory,
  // and one extra line is immaterial next to 5,000, whereas letting it consume a
  // slot would make "the last maxLines lines" quietly untrue.
  if (copied) {
    const prev = out.length > 0 && out[0].stream === NOTICE_STREAM ? out[0] : null
    const real = prev ? out.length - 1 : out.length
    if (real > maxLines) {
      out = [
        truncationNotice((prev ? prev.dropped : 0) + (real - maxLines), maxLines),
        ...out.slice(out.length - maxLines),
      ]
    }
  }
  return out
}
