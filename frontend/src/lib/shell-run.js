import { StartShellRun, CancelShellRun } from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

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
