import { SearchGraph } from '../../wailsjs/go/main/App'

// runCompute spawns a fresh worker, bridges graph.query/orbit to SearchGraph,
// enforces a timeout, and streams output via callbacks. Returns { cancel }.
export function runCompute(code, { onConsole = () => {}, onValue = () => {}, onChart = () => {}, onError = () => {}, onDone = () => {}, timeoutMs = 5000 } = {}) {
  const worker = new Worker(new URL('./compute-worker.js', import.meta.url), { type: 'module' })
  let finished = false

  const timer = setTimeout(() => finish('timeout'), timeoutMs)

  function finish(reason) {
    if (finished) return
    finished = true
    clearTimeout(timer)
    worker.terminate()
    if (reason === 'timeout') onError(`Timed out after ${timeoutMs}ms (terminated).`)
    onDone()
  }

  worker.onmessage = async (e) => {
    const m = e.data
    if (finished) return
    switch (m.type) {
      case 'console': onConsole(m.level, m.text); break
      case 'chart': onChart(m.config); break
      case 'value': onValue(m.text); break
      case 'error': onError(m.text); finish('error'); break
      case 'done': finish('done'); break
      case 'graph-req': {
        try {
          const result = await SearchGraph(m.query)
          if (!finished) worker.postMessage({ type: 'graph-res', id: m.id, result })
        } catch (err) {
          if (!finished) worker.postMessage({ type: 'graph-res', id: m.id, error: String((err && err.message) || err) })
        }
        break
      }
    }
  }
  worker.onerror = (e) => { onError(e.message || 'compute worker failed'); finish('error') }

  worker.postMessage({ type: 'run', code })
  return { cancel: () => finish('cancel') }
}
