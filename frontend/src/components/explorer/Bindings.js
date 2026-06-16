// Thin wrapper around the Wails-bound explorer methods.
//
// We don't import from frontend/wailsjs/go/main/App.js because that file is
// auto-generated and may not yet contain the explorer methods until the next
// `wails dev` / `wails build` regenerates it. The Wails runtime injects ALL
// bound methods on `window.go.main.App` at startup regardless of the
// generated file's contents, so reaching for the global is the most
// regeneration-tolerant path during foundational work.
//
// On the next regeneration the App.js/App.d.ts files will contain the same
// methods and a thin import wrapper would be a drop-in replacement.

/**
 * Open the explorer with a query against the SSTorytime-KV graph.
 * Returns a LayoutResult per contracts/layout-types.md.
 * @param {string} query
 * @param {'stacked' | 'extended'} [mode='stacked']
 * @returns {Promise<any>}
 */
export async function explorerOpen(query, mode = 'stacked') {
  const fn = window?.go?.main?.App?.ExplorerOpen
  if (typeof fn !== 'function') {
    throw new Error('ExplorerOpen binding not available — restart `wails dev` to regenerate bindings')
  }
  return fn(query, mode)
}

/**
 * Re-anchor the active result on the supplied node. Also doubles as
 * the layout-mode flip path: pass the current resultID and anchorID
 * with a different mode to swap encodings without re-running the
 * search.
 * @param {string} resultID
 * @param {string} anchorID
 * @param {'stacked' | 'extended'} [mode='stacked']
 * @returns {Promise<any>}
 */
export async function explorerReanchor(resultID, anchorID, mode = 'stacked') {
  const fn = window?.go?.main?.App?.ExplorerReanchor
  if (typeof fn !== 'function') {
    throw new Error('ExplorerReanchor binding not available — restart `wails dev` to regenerate bindings')
  }
  return fn(resultID, anchorID, mode)
}

/** Drop the cached ResultSet on the Go side. */
export async function explorerClose() {
  const fn = window?.go?.main?.App?.ExplorerClose
  if (typeof fn !== 'function') return // non-fatal: nothing to close
  return fn()
}
