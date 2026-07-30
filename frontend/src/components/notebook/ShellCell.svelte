<script>
  import { onDestroy, onMount } from 'svelte'
  import { ListShells } from '../../../wailsjs/go/main/App'
  import { runShell } from '../../lib/shell-run.js'
  import { appendChunks } from '../../lib/shell-lines.js'

  export let cell
  export let onChange = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused
  export let onInsert = () => {}

  let shells = []
  let discoverErr = ''
  let lines = []          // scrollback — component-local, never persisted
  let running = false
  let run = null
  let status = ''         // "exit 0 · 0.4s", "cancelled", or a spawn error
  let statusBad = false

  onMount(async () => {
    try {
      shells = await ListShells()
      if (!shells.length) { discoverErr = 'No shell found on this system.'; return }
      // Preselect the first discovered shell, or restore a saved choice that is
      // still installed. A saved shell that has gone away must not silently
      // become a different one.
      const saved = shells.find((s) => s.id === cell.shell)
      if (!saved) selectShell(shells[0])
      else if (saved.label !== cell.shellLabel) selectShell(saved)
    } catch (e) { discoverErr = String(e?.message ?? e) }
  })

  onDestroy(() => run?.cancel())

  function selectShell(s) {
    cell.shell = s.id
    cell.shellLabel = s.label
    onChange(cell)
  }
  function onPickShell(e) {
    const s = shells.find((x) => x.id === e.target.value)
    if (s) selectShell(s)
  }
  function onCwd(e) { cell.cwd = e.target.value; onChange(cell) }
  function onCommand(e) { cell.command = e.target.value; onChange(cell) }

  // Ctrl/Cmd+Enter runs, matching LLMCell's send shortcut.
  function onKey(e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); start() }
  }

  function start() {
    if (readOnly || running || !cell.command?.trim() || !cell.shell) return
    lines = []
    status = ''
    statusBad = false
    running = true
    run = runShell({
      shellId: cell.shell, cwd: cell.cwd, command: cell.command,
      onChunks: (chunks) => { lines = appendChunks(lines, chunks) },
      onExit: (code, ms) => {
        running = false; run = null
        // A non-zero exit is information, not a failure: it belongs here, not in
        // an error toast. `grep` returning 1 is normal.
        status = code === -1 ? 'cancelled' : `exit ${code} · ${(ms / 1000).toFixed(1)}s`
        statusBad = code !== 0
      },
      onError: (message) => {
        running = false; run = null
        status = message; statusBad = true
      },
    })
  }
  function stop() { run?.cancel(); run = null; running = false; status = 'cancelled'; statusBad = true }
</script>

<div class="sh-cell">
  {#if discoverErr}
    <div class="sh-error">{discoverErr}</div>
  {:else}
    {#if !readOnly}
      <div class="sh-bar">
        <select class="sh-shell" value={cell.shell} on:change={onPickShell} disabled={running}>
          {#each shells as s (s.id)}<option value={s.id}>{s.label}</option>{/each}
        </select>
        <input class="sh-cwd" type="text" placeholder="working directory"
          value={cell.cwd ?? ''} on:input={onCwd} disabled={running} />
        <span class="sh-spacer"></span>
        {#if running}
          <button class="sh-stop" on:click={stop}>Stop</button>
        {:else}
          <button class="sh-run" on:click={start} disabled={!cell.command?.trim()}>Run</button>
        {/if}
      </div>
    {/if}

    <textarea class="sh-cmd" value={cell.command ?? ''} on:input={onCommand} on:keydown={onKey}
      disabled={readOnly} spellcheck="false" rows="2"
      placeholder={'docker ps --format \'{{.Names}}\'\nCtrl+Enter to run'}></textarea>

    {#if lines.length || status}
      <div class="sh-out">
        {#each lines as l, i (i)}<div class="sh-line" class:err={l.stream === 'stderr'}>{l.text}</div>{/each}
        {#if status}<div class="sh-status" class:bad={statusBad}>{status}</div>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .sh-cell { border: 1px solid #2a3a4a; border-radius: 8px; background: #0e1626; overflow: hidden; }
  .sh-bar { display: flex; align-items: center; gap: 6px; padding: 5px 8px; border-bottom: 1px solid #2a3a4a; }
  .sh-shell, .sh-cwd { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 6px; font-size: 12px; }
  .sh-cwd { flex: 1; min-width: 0; }
  .sh-spacer { flex: none; }
  .sh-run, .sh-stop { background: #14233a; color: #93c5fd; border: 1px solid #2a4a6a; border-radius: 6px; padding: 2px 10px; cursor: pointer; font-size: 12px; }
  .sh-run:disabled { opacity: .45; cursor: default; }
  .sh-stop { color: #ff8080; border-color: #5a2a2a; }
  .sh-cmd { width: 100%; box-sizing: border-box; background: #0b1220; color: #e8eaf0; border: none; border-bottom: 1px solid #2a3a4a; padding: 6px 8px; font-family: ui-monospace, monospace; font-size: 12px; resize: vertical; }
  .sh-out { max-height: 320px; overflow: auto; padding: 6px 8px; font-family: ui-monospace, monospace; font-size: 12px; }
  .sh-line { white-space: pre-wrap; word-break: break-word; color: #c9d4e3; }
  .sh-line.err { color: #ffb4a0; }
  .sh-status { margin-top: 4px; color: #5d7186; font-size: 11px; }
  .sh-status.bad { color: #ff8080; }
  .sh-error { padding: 8px; color: #ff8080; font-size: 12px; }
</style>
