<script>
  import { onDestroy } from 'svelte'
  import { runCompute } from '../../lib/compute-host.js'
  import Chart from 'chart.js/auto'

  export let cell
  export let onChange = () => {}
  export let readOnly = false
  export let context = null // unused; shared cell prop set
  export let cells = []      // unused
  export let onPush = () => {} // unused; shared cell prop set

  let running = false
  let logs = []        // { level, text }
  let value = ''
  let error = ''
  let chartConfigs = []
  let run = null

  onDestroy(() => run?.cancel())

  function persist() {
    const text = [...logs.map((l) => l.text), value, error].filter(Boolean).join('\n').slice(0, 4000)
    cell.resultText = text
    onChange(cell)
  }

  function start() {
    if (running) return
    logs = []; value = ''; error = ''; chartConfigs = []
    running = true
    run = runCompute(cell.code ?? '', {
      onConsole: (level, text) => { logs = [...logs, { level, text }] },
      onValue: (text) => { value = text },
      onChart: (config) => { chartConfigs = [...chartConfigs, config] },
      onError: (text) => { error = text },
      onDone: () => { running = false; run = null; persist() },
    })
  }
  function stop() { run?.cancel(); running = false; run = null }
  function onCodeInput(e) { cell.code = e.target.value; onChange(cell) }

  // Svelte action: render a Chart.js chart into the canvas, destroy on teardown.
  function chartAction(node, config) {
    const c = new Chart(node, config)
    return { destroy() { c.destroy() } }
  }
</script>

<div class="cc-cell">
  {#if !readOnly}
    <div class="cc-bar">
      <span class="cc-spacer"></span>
      {#if running}<button class="cc-stop" on:click={stop}>Stop</button>{:else}<button class="cc-run" on:click={start}>Run</button>{/if}
    </div>
  {/if}
  <textarea class="cc-code" value={cell.code ?? ''} on:input={onCodeInput} disabled={readOnly} spellcheck="false"
    placeholder={"// math.js as `math`, async `graph.query(q)` / `graph.orbit(node)`, `chart(config)`.\n// `print(x)` shows a value; or `return` one.\nprint(math.sqrt(16))"} rows="5"></textarea>

  {#if logs.length || value || error || chartConfigs.length}
    <div class="cc-out">
      {#each logs as l}<div class="cc-log {l.level}">{l.text}</div>{/each}
      {#if value}<div class="cc-value">⇒ {value}</div>{/if}
      {#if error}<div class="cc-error">{error}</div>{/if}
      {#each chartConfigs as cfg (cfg)}<div class="cc-chart"><canvas use:chartAction={cfg}></canvas></div>{/each}
    </div>
  {/if}
</div>

<style>
  .cc-cell { background: var(--surface-modal); border: 1px solid var(--border-card); border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .cc-bar { display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-bottom: 1px solid var(--border-subtle); }
  .cc-spacer { flex: 1; }
  .cc-run { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .cc-stop { background: var(--danger-fill); color: var(--danger-fill-text); border: 1px solid var(--border); border-radius: 6px; padding: 3px 12px; cursor: pointer; font-size: 12px; }
  .cc-code { width: 100%; box-sizing: border-box; background: var(--surface-sunken); border: none; color: var(--text); font-family: monospace; font-size: 13px; padding: 8px; resize: vertical; }
  .cc-out { padding: 8px; border-top: 1px solid var(--border-subtle); display: flex; flex-direction: column; gap: 4px; }
  .cc-log { font-family: monospace; font-size: 12px; color: var(--text-cell-muted); white-space: pre-wrap; }
  .cc-log.warn { color: var(--warn-output); }
  .cc-log.error { color: var(--danger); }
  .cc-log.print { color: var(--accent-text); }
  .cc-value { font-family: monospace; font-size: 13px; color: var(--accent-text); white-space: pre-wrap; }
  .cc-error { color: var(--danger); font-size: 12px; white-space: pre-wrap; }
  .cc-chart { background: var(--surface-sunken); border: 1px solid var(--border-subtle); border-radius: 6px; padding: 6px; margin-top: 4px; }
</style>
