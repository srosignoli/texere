// A debounced save for the notebook. Kept out of NotebookPane so its rules can
// be tested: a burst of changes saves once, and a failed save is reported
// rather than swallowed.

// save() resolves when the save is done and rejects when it failed.
export function createAutosave({
  save, delayMs = 600, onError = () => {}, onSaved = () => {},
  // Wrapped, not passed bare: called as timers.set(...), a bare setTimeout
  // gets `this` = timers, which the webview rejects.
  timers = { set: (fn, ms) => setTimeout(fn, ms), clear: (id) => clearTimeout(id) },
}) {
  let timer = null

  async function run() {
    try {
      await save()
      onSaved()
    } catch (e) {
      onError(e)
    }
  }

  return {
    schedule() {
      if (timer != null) timers.clear(timer)
      timer = timers.set(() => { timer = null; return run() }, delayMs)
    },
    // flush saves now if a save is pending, and does nothing otherwise.
    async flush() {
      if (timer == null) return
      timers.clear(timer)
      timer = null
      await run()
    },
  }
}
