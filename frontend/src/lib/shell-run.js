import { StartShellRun, CancelShellRun } from '../../wailsjs/go/main/App.js'
import { EventsOn } from '../../wailsjs/runtime/runtime.js'

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

  for (const chunk of chunks) {
    const text = String(chunk?.text ?? '').replace(/\r/g, '')
    if (!text) continue
    ensureCopy()

    const stream = chunk?.stream === 'stderr' ? 'stderr' : 'stdout'
    const parts = text.split('\n')
    const tail = out.length ? out[out.length - 1] : null

    // parts[0] continues an open line of the same stream, if there is one.
    if (tail && tail.open && tail.stream === stream) {
      out[out.length - 1] = { ...tail, text: tail.text + parts[0] }
    } else if (parts[0] !== '' || parts.length === 1) {
      out.push({ stream, text: parts[0], open: true })
    }

    for (let i = 1; i < parts.length; i++) {
      // The line before each newline is now closed.
      if (out.length) out[out.length - 1] = { ...out[out.length - 1], open: false }
      const isLast = i === parts.length - 1
      // A trailing newline leaves nothing open; anything after it starts a new
      // open line.
      if (!isLast || parts[i] !== '') out.push({ stream, text: parts[i], open: true })
    }
  }

  if (copied && out.length > maxLines) out = out.slice(out.length - maxLines)
  return out
}

// runShell starts a run and wires the out/exit/error events, filtered to this
// run's id. Returns a cancel() that stops the run and unsubscribes.
//
// The `cancelled` flag handles Stop being pressed before StartShellRun resolves
// — without it the process would keep running with nothing tracking its id.
// Copied from streamTurn in llm.js, which solves the same race.
export function runShell({ shellId, cwd, command, onChunks, onExit, onError }) {
  let offOut = () => {}
  let offExit = () => {}
  let offError = () => {}
  let rid = null
  let cancelled = false

  StartShellRun({ shellId: shellId || '', cwd: cwd || '', command })
    .then((runId) => {
      rid = runId
      if (cancelled) { CancelShellRun(rid); return }
      offOut = EventsOn('shell:out', (p) => { if (p?.runId === rid) onChunks(p.chunks ?? []) })
      offExit = EventsOn('shell:exit', (p) => {
        if (p?.runId === rid) { cleanup(); onExit(p.exitCode, p.durationMs) }
      })
      offError = EventsOn('shell:error', (p) => {
        if (p?.runId === rid) { cleanup(); onError(p.message) }
      })
    })
    .catch((e) => onError(String(e?.message ?? e)))

  function cleanup() { offOut(); offExit(); offError() }
  function cancel() { cancelled = true; if (rid) CancelShellRun(rid); cleanup() }
  return { cancel }
}
