// JS-kernel Web Worker for the compute cell. Runs the user's code as the body
// of an async function with `math`, `graph`, `chart`, and `console` in scope.
import { create, all } from 'mathjs'

const math = create(all)
let reqId = 0
const pending = new Map()

function post(msg) { self.postMessage(msg) }
function fmt(v) {
  if (typeof v === 'string') return v
  try { return JSON.stringify(v) } catch { return String(v) }
}

function rpc(query) {
  const id = ++reqId
  post({ type: 'graph-req', id, query })
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
}
const graph = {
  query: (q) => rpc(String(q)),
  orbit: (node, radius = 1) => rpc(String(node) + ' \\radius ' + radius),
}
const chart = (config) => post({ type: 'chart', config })
// print(...) shows a value in the output without needing `return`; it rides the
// console channel with a distinct 'print' level the UI styles like the value.
const print = (...a) => post({ type: 'console', level: 'print', text: a.map(fmt).join(' ') })
const consoleShim = {
  log: (...a) => post({ type: 'console', level: 'log', text: a.map(fmt).join(' ') }),
  info: (...a) => post({ type: 'console', level: 'info', text: a.map(fmt).join(' ') }),
  warn: (...a) => post({ type: 'console', level: 'warn', text: a.map(fmt).join(' ') }),
  error: (...a) => post({ type: 'console', level: 'error', text: a.map(fmt).join(' ') }),
}

self.onmessage = async (e) => {
  const m = e.data
  if (m.type === 'graph-res') {
    const p = pending.get(m.id)
    if (!p) return
    pending.delete(m.id)
    if (m.error) p.reject(new Error(m.error))
    else p.resolve(m.result)
    return
  }
  if (m.type === 'run') {
    try {
      // eslint-disable-next-line no-new-func
      const fn = new Function('math', 'graph', 'chart', 'print', 'console',
        '"use strict"; return (async () => {\n' + m.code + '\n})();')
      const value = await fn(math, graph, chart, print, consoleShim)
      if (value !== undefined) post({ type: 'value', text: fmt(value) })
      post({ type: 'done' })
    } catch (err) {
      post({ type: 'error', text: String((err && err.message) || err) })
    }
  }
}
