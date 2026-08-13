<script>
  import { createEventDispatcher } from 'svelte'
  import VirtualList from './VirtualList.svelte'
  import { logEntries, logPanelOpen, clearLog } from '../stores/logPanel.js'

  const dispatch = createEventDispatcher()

  // Severity filter — chip-style toggle. 'all' shows every entry.
  let severityFilter = 'all'

  $: filtered = (() => {
    const all = $logEntries
    if (severityFilter === 'all') return all
    return all.filter((e) => e.severity === severityFilter)
  })()

  $: counts = (() => {
    let err = 0, warn = 0, info = 0
    for (const e of $logEntries) {
      if (e.severity === 'error') err++
      else if (e.severity === 'warning') warn++
      else info++
    }
    return { err, warn, info, total: $logEntries.length }
  })()

  function severityIcon(s) {
    if (s === 'error') return '✕'
    if (s === 'warning') return '!'
    return 'i'
  }

  function shortPath(p) {
    if (!p) return ''
    const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
    return i === -1 ? p : p.slice(i + 1)
  }

  function onRowClick(entry) {
    dispatch('entry-click', entry)
  }

  function close() {
    logPanelOpen.set(false)
  }

  // Format a single entry as a compact text line for clipboard / file paste.
  function formatEntry(e) {
    const sev = e.severity?.toUpperCase().padEnd(7, ' ') ?? '       '
    const loc = e.fileName ?? (e.filePath ? shortPath(e.filePath) : '')
    const locStr = loc ? `${loc}${e.line ? ':' + e.line : ''}\t` : ''
    const src = e.source ? `[${e.source}]\t` : ''
    return `${sev}\t${src}${locStr}${e.message ?? ''}`
  }

  let copyBadge = '' // transient "copied!" feedback
  async function copyAll() {
    const text = filtered.map(formatEntry).join('\n')
    try {
      await navigator.clipboard.writeText(text)
      copyBadge = `copied ${filtered.length}`
    } catch (err) {
      // navigator.clipboard may be unavailable in some webview contexts;
      // fall back to a temporary <textarea> select+copy.
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      try { document.execCommand('copy'); copyBadge = `copied ${filtered.length}` }
      catch { copyBadge = 'copy failed' }
      document.body.removeChild(ta)
    }
    setTimeout(() => { copyBadge = '' }, 1500)
  }
</script>

<div class="log-panel">
  <div class="log-header">
    <span class="log-title">Logs</span>
    <span class="log-counts">
      {#if counts.err}<span class="count count-err" title="Errors">✕ {counts.err}</span>{/if}
      {#if counts.warn}<span class="count count-warn" title="Warnings">! {counts.warn}</span>{/if}
      {#if counts.info}<span class="count count-info" title="Info">i {counts.info}</span>{/if}
      {#if counts.total === 0}<span class="count count-empty">empty</span>{/if}
    </span>
    <div class="filter-chips">
      <button class="filter" class:active={severityFilter === 'all'} on:click={() => (severityFilter = 'all')}>all</button>
      <button class="filter" class:active={severityFilter === 'error'} on:click={() => (severityFilter = 'error')}>errors</button>
      <button class="filter" class:active={severityFilter === 'warning'} on:click={() => (severityFilter = 'warning')}>warnings</button>
    </div>
    <span class="spacer" />
    {#if copyBadge}<span class="copy-badge">{copyBadge}</span>{/if}
    <button class="action" on:click={copyAll} title="Copy filtered entries to clipboard">copy</button>
    <button class="action" on:click={clearLog} title="Clear all entries">clear</button>
    <button class="action close" on:click={close} title="Close (Esc)">✕</button>
  </div>

  <div class="log-body">
    {#if filtered.length === 0}
      <div class="log-empty">{counts.total === 0 ? 'No log entries yet.' : 'No entries match the current filter.'}</div>
    {:else}
      <VirtualList items={filtered} estimatedHeight={26} overscan={6} let:item>
        <button
          class="log-row sev-{item.severity} src-{item.source}"
          on:click={() => onRowClick(item)}
          title={item.message}
        >
          <span class="sev" aria-label={item.severity}>{severityIcon(item.severity)}</span>
          {#if item.fileName || item.filePath}
            <span class="loc">
              {item.fileName ?? shortPath(item.filePath)}{#if item.line}:{item.line}{/if}
            </span>
          {/if}
          <span class="msg">{item.message}</span>
          <span class="src">{item.source}</span>
        </button>
      </VirtualList>
    {/if}
  </div>
</div>

<style>
  .log-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
    background: #131c2a;
    color: var(--text-secondary);
    overflow: hidden;
  }

  .log-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 10px;
    background: var(--surface-overlay);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    font-size: 0.78rem;
  }

  .log-title {
    font-weight: 600;
    color: #8aa0b0;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    font-size: 0.7rem;
  }

  .log-counts {
    display: flex;
    gap: 6px;
    align-items: baseline;
  }
  .count {
    font-variant-numeric: tabular-nums;
    font-size: 0.72rem;
  }
  .count-err { color: #d96a6a; }
  .count-warn { color: #d9b06a; }
  .count-info { color: #6a9aaa; }
  .count-empty { color: var(--text-hint); font-style: italic; }

  .filter-chips {
    display: flex;
    gap: 4px;
  }
  .filter {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 10px;
    color: #8aa0b0;
    padding: 1px 8px;
    font-size: 0.7rem;
    cursor: pointer;
    font-family: inherit;
    transition: background 0.08s, color 0.08s, border-color 0.08s;
  }
  .filter:hover {
    color: var(--text-secondary);
    border-color: var(--accent-faint);
  }
  .filter.active {
    background: #1e3242;
    color: #d8c590;
    border-color: #c9a96e;
  }

  .spacer { flex: 1; }

  .copy-badge {
    color: #6a9aaa;
    font-size: 0.7rem;
    font-style: italic;
    flex-shrink: 0;
  }

  .action {
    background: transparent;
    border: none;
    color: #6a8aa0;
    font-size: 0.72rem;
    cursor: pointer;
    padding: 2px 6px;
    font-family: inherit;
  }
  .action:hover { color: var(--text); }
  .action.close { font-size: 0.85rem; }

  .log-body {
    flex: 1;
    min-height: 0;
    position: relative;
  }

  .log-empty {
    padding: 12px 14px;
    color: var(--text-hint);
    font-style: italic;
    font-size: 0.78rem;
  }

  .log-row {
    display: flex;
    align-items: baseline;
    gap: 8px;
    width: 100%;
    padding: 3px 12px;
    background: none;
    border: none;
    color: inherit;
    font-family: inherit;
    font-size: 0.78rem;
    text-align: left;
    cursor: pointer;
    border-left: 2px solid transparent;
    /* Allow drag-select inside the row so the user can grab message text
       directly. The "copy" header button is still the easy path for
       copying every visible entry; this just makes single-line copy work. */
    user-select: text;
  }
  .log-row .loc,
  .log-row .msg {
    user-select: text;
    -webkit-user-select: text;
  }
  .log-row:hover {
    background: var(--surface-overlay);
  }
  .log-row.sev-error { border-left-color: #d96a6a; }
  .log-row.sev-warning { border-left-color: #d9b06a; }
  .log-row.sev-info { border-left-color: #6a9aaa; }

  .sev {
    flex-shrink: 0;
    width: 14px;
    text-align: center;
    font-weight: 700;
  }
  .sev-error .sev { color: #d96a6a; }
  .sev-warning .sev { color: #d9b06a; }
  .sev-info .sev { color: #6a9aaa; }

  .loc {
    flex-shrink: 0;
    color: #7da0c0;
    font-family: monospace;
    font-size: 0.74rem;
  }
  .msg {
    flex: 1;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .src {
    flex-shrink: 0;
    color: var(--text-hint);
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }
</style>
