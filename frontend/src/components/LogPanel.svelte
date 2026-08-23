<script>
  import { createEventDispatcher } from 'svelte'
  import VirtualList from './VirtualList.svelte'
  import { logEntries, logPanelOpen, clearLog } from '../stores/logPanel.js'
  import { lintResult } from '../stores/lint.js'

  const dispatch = createEventDispatcher()

  // Log vs Problems: two different kinds of content sharing one panel shell.
  // logEntries is an append-only history; lintResult is current state,
  // replaced wholesale on every lint run. They must never share storage —
  // that's the entire reason this is a tab switch and not a filter chip.
  let panelTab = 'log' // 'log' | 'problems'

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

  // ----- Problems (Defects + Vocabulary) -----

  $: defects = $lintResult.defects ?? []
  $: vocabulary = $lintResult.vocabulary ?? [] // already sorted by uses descending

  // Group defects by file, preserving the File-then-Line order LintCollection
  // already sorted them in — no re-sort needed here.
  $: defectGroups = (() => {
    const byFile = new Map()
    for (const d of defects) {
      if (!byFile.has(d.file)) byFile.set(d.file, [])
      byFile.get(d.file).push(d)
    }
    return [...byFile.entries()].map(([file, items]) => ({ file, items }))
  })()

  $: problemsTotal = defects.length + vocabulary.length
  $: maxVocabUses = vocabulary.length ? vocabulary[0].uses : 0

  // Bar width is linear against the largest use count on purpose: on the real
  // collection the top two arrows dominate the long tail, and a linear scale
  // is what actually shows that at a glance. A log scale would flatten it.
  function vocabBarPct(uses) {
    return maxVocabUses > 0 ? (uses / maxVocabUses) * 100 : 0
  }

  function filesSummary(files) {
    const n = files?.length ?? 0
    return `${n} ${n === 1 ? 'file' : 'files'}`
  }

  // The Msg the parser emits already leads with "<kind>: <detail>" (kindOf's
  // own prefix). The kind is shown as its own badge, so strip the redundant
  // repeat rather than printing it twice — display formatting only, derives
  // no new information the ParseError didn't already carry.
  function defectDetail(d) {
    const prefix = `${d.kind}: `
    return d.message.startsWith(prefix) ? d.message.slice(prefix.length) : d.message
  }

  function switchTab(tab) {
    panelTab = tab
  }

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
  <div class="panel-tabbar" role="tablist">
    <button class="ptab" class:active={panelTab === 'log'} role="tab" aria-selected={panelTab === 'log'} on:click={() => switchTab('log')}>
      Log
      {#if counts.total}<span class="ptab-count">{counts.total}</span>{/if}
    </button>
    <button class="ptab" class:active={panelTab === 'problems'} role="tab" aria-selected={panelTab === 'problems'} on:click={() => switchTab('problems')}>
      Problems
      <!-- Defects only, deliberately — same convention as ColumnChapters' per-file
           !N marker (Task 3): undeclared arrows are not defects, and mixing them
           in here would both blur that distinction and pin a count that may never
           reach zero (many arrows are legitimately never declared). -->
      {#if defects.length}<span class="ptab-count ptab-count-danger">{defects.length}</span>{/if}
    </button>
  </div>

  {#if panelTab === 'log'}
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
  {:else}
  <div class="log-header">
    <span class="log-title">Problems</span>
    <span class="log-counts">
      {#if defects.length}<span class="count count-err" title="Defects">! {defects.length}</span>{/if}
      {#if vocabulary.length}<span class="count count-warn" title="Undeclared arrows">! {vocabulary.length}</span>{/if}
      {#if problemsTotal === 0}<span class="count count-empty">clean</span>{/if}
    </span>
    <span class="spacer" />
    <button class="action close" on:click={close} title="Close (Esc)">✕</button>
  </div>

  <div class="problems-body">
    {#if problemsTotal === 0}
      <div class="log-empty">
        {$lintResult.filesLinted === 0 ? 'Nothing linted yet.' : 'No problems found — this collection is clean.'}
      </div>
    {:else}
      <div class="problems-columns">
        <section class="problems-col">
          <div class="col-head">
            <h3>Defects</h3>
            <span class="col-count">{defects.length} across {defectGroups.length} {defectGroups.length === 1 ? 'file' : 'files'}</span>
          </div>
          {#if defectGroups.length === 0}
            <div class="col-empty">No defects.</div>
          {:else}
            <div class="col-scroll">
              {#each defectGroups as group (group.file)}
                <div class="defect-group">
                  <div class="defect-file">
                    <span class="defect-file-name">{group.file}</span>
                    <span class="defect-file-count">{group.items.length}</span>
                  </div>
                  {#each group.items as d}
                    <div class="defect-row">
                      <div class="defect-meta">
                        <span class="defect-loc">{d.line > 0 ? `line ${d.line}` : 'file-level'}</span>
                        <span class="defect-kind">{d.kind}</span>
                      </div>
                      <div class="defect-msg">{defectDetail(d)}</div>
                      {#if d.remediation}<div class="defect-remediation">{d.remediation}</div>{/if}
                    </div>
                  {/each}
                </div>
              {/each}
            </div>
          {/if}
        </section>

        <section class="problems-col">
          <div class="col-head">
            <h3>Vocabulary</h3>
            <span class="col-count">{vocabulary.length} undeclared {vocabulary.length === 1 ? 'arrow' : 'arrows'}</span>
          </div>
          {#if vocabulary.length === 0}
            <div class="col-empty">No undeclared arrows.</div>
          {:else}
            <p class="vocab-lead">Not declared in SSTconfig, so these relations don't become graph edges yet. Declare the arrow to fix — every row below shares that one remediation.</p>
            <div class="col-scroll vocab-scroll">
              <VirtualList items={vocabulary} estimatedHeight={22} overscan={8} getKey={(item) => item.arrow} let:item>
                <div class="vocab-row" title="{item.arrow} — {item.files.length} {item.files.length === 1 ? 'file' : 'files'}: {item.files.join(', ')}">
                  <span class="vocab-arrow">{item.arrow}</span>
                  <span class="vocab-bar-track"><span class="vocab-bar-fill" style="width: {vocabBarPct(item.uses)}%"></span></span>
                  <span class="vocab-uses">{item.uses.toLocaleString()}</span>
                  <span class="vocab-files">{filesSummary(item.files)}</span>
                </div>
              </VirtualList>
            </div>
          {/if}
        </section>
      </div>
    {/if}
  </div>
  {/if}
</div>

<style>
  .log-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
    background: var(--surface-panel);
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
    color: var(--chip-text-dim);
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
  /* Severity colours. err/warn map to --danger/--warn (the app-wide status
     hues), per this task's brief — not byte-identical to the literals they
     replace (#d96a6a/#d9b06a vs --danger #ff8080/--warn #f59e0b), but the
     brief calls this out explicitly: a log severity should read as the same
     red/amber the rest of the app already uses, rather than keep its own
     one-off pair. info has no matching status hue (--ok means success, not
     "informational"), so it gets its own --sev-info token (review fix,
     round 1: reusing --text-arrow-hint — documented as OmniBar's arrow-
     glyph colour — would have meant retuning OmniBar's own colour silently
     moved this severity too; the brief's "reuse the triad, don't invent"
     only covers danger/warn/ok). --sev-info is anchored to the literal it
     replaces (#6a9aaa) in Blue/Dark and re-solved for contrast in Light —
     see style.css. */
  .count-err { color: var(--danger); }
  .count-warn { color: var(--warn); }
  .count-info { color: var(--sev-info); }
  .count-empty { color: var(--text-hint); font-style: italic; }

  .filter-chips {
    display: flex;
    gap: 4px;
  }
  .filter {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 10px;
    color: var(--chip-text-dim);
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
    background: var(--log-filter-active-bg);
    color: var(--log-filter-active-text);
    border-color: var(--log-filter-active-border);
  }

  .spacer { flex: 1; }

  .copy-badge {
    color: var(--sev-info);
    font-size: 0.7rem;
    font-style: italic;
    flex-shrink: 0;
  }

  .action {
    background: transparent;
    border: none;
    color: var(--text-inert);
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
  .log-row.sev-error { border-left-color: var(--danger); }
  .log-row.sev-warning { border-left-color: var(--warn); }
  .log-row.sev-info { border-left-color: var(--sev-info); }

  .sev {
    flex-shrink: 0;
    width: 14px;
    text-align: center;
    font-weight: 700;
  }
  .sev-error .sev { color: var(--danger); }
  .sev-warning .sev { color: var(--warn); }
  .sev-info .sev { color: var(--sev-info); }

  .loc {
    flex-shrink: 0;
    color: var(--text-loc);
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

  /* ----- Tab bar (Log / Problems) ----- */

  .panel-tabbar {
    display: flex;
    gap: 2px;
    padding: 0 10px;
    background: var(--surface-overlay);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }
  .ptab {
    display: flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--chip-text-dim);
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    padding: 6px 10px 5px;
    cursor: pointer;
    font-family: inherit;
    transition: color 0.08s, border-color 0.08s;
  }
  .ptab:hover { color: var(--text-secondary); }
  .ptab.active { color: var(--text); border-bottom-color: var(--accent-primary); }
  .ptab-count {
    font-variant-numeric: tabular-nums;
    font-size: 0.68rem;
    color: var(--text-muted);
    text-transform: none;
    letter-spacing: normal;
  }
  .ptab-count.ptab-count-danger { color: var(--danger); }

  /* ----- Problems view ----- */

  .problems-body {
    flex: 1;
    min-height: 0;
    position: relative;
    overflow: hidden;
  }

  .problems-columns {
    display: flex;
    height: 100%;
    min-height: 0;
  }

  .problems-col {
    flex: 1 1 0;
    min-width: 0;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
  }
  .problems-col:last-child { border-right: none; }

  .col-head {
    display: flex;
    align-items: baseline;
    gap: 8px;
    padding: 8px 12px 4px;
    flex-shrink: 0;
  }
  .col-head h3 {
    margin: 0;
    font-size: 0.72rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--accent-dim);
  }
  .col-count {
    font-size: 0.7rem;
    color: var(--text-muted);
    font-variant-numeric: tabular-nums;
  }
  .col-empty {
    padding: 6px 12px 12px;
    color: var(--text-hint);
    font-style: italic;
    font-size: 0.76rem;
  }
  .col-scroll {
    flex: 1;
    min-height: 0;
    overflow-y: auto;
  }

  .vocab-lead {
    margin: 0 12px 6px;
    color: var(--help-lead-text);
    font-size: 0.72rem;
    line-height: 1.35;
  }
  .vocab-scroll { min-height: 0; }

  /* Defects: grouped by file, sticky file header so the group stays
     identifiable while scrolling through a file with many findings. */
  .defect-group {
    border-top: 1px solid var(--help-divider);
  }
  .defect-group:first-child { border-top: none; }

  .defect-file {
    position: sticky;
    top: 0;
    z-index: 1;
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
    padding: 4px 12px;
    background: var(--surface-block);
    font-family: monospace;
    font-size: 0.74rem;
  }
  .defect-file-name { color: var(--chapter-file-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .defect-file-count { flex-shrink: 0; color: var(--danger); font-weight: 700; }

  .defect-row {
    padding: 6px 12px 8px 16px;
    border-top: 1px solid var(--border-faint);
  }
  .defect-meta {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .defect-loc {
    flex-shrink: 0;
    color: var(--text-loc);
    font-family: monospace;
    font-size: 0.72rem;
  }
  .defect-kind {
    color: var(--text-badge);
    font-size: 0.7rem;
    text-transform: capitalize;
  }
  .defect-msg {
    margin-top: 2px;
    color: var(--text-secondary);
    font-size: 0.78rem;
    line-height: 1.35;
  }
  .defect-remediation {
    margin-top: 4px;
    padding: 4px 8px;
    background: var(--surface-block);
    border-radius: 4px;
    color: var(--text-block);
    font-size: 0.74rem;
    line-height: 1.35;
  }

  /* Vocabulary: one row per undeclared arrow, sorted by use count descending.
     The bar is linear against the largest count on purpose — on the real
     collection two arrows dominate a long tail of 150+ others, and a linear
     scale is what actually shows that at a glance. */
  .vocab-row {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 2px 12px;
    font-size: 0.76rem;
  }
  /* Wraps rather than ellipsizing: this section's entire job is telling the
     user which arrow to declare, so the name must always be fully readable,
     not just recoverable via title. Most arrows are short (~120px worth,
     one line); the ~22% that run longer (e.g. "ctx PoSST,arrow ArrowPtr")
     wrap to a second line instead of losing characters — VirtualList
     measures each row's real height via ResizeObserver, so a taller
     wrapped row doesn't misalign anything around it. Also carried in the
     row's title (arrow first, then file list) for a copy-pastable form. */
  .vocab-arrow {
    flex: 0 1 200px;
    min-width: 90px;
    max-width: 240px;
    overflow-wrap: anywhere;
    white-space: normal;
    color: var(--help-code-text);
    font-family: monospace;
    font-size: 0.74rem;
    line-height: 1.3;
  }
  .vocab-bar-track {
    flex: 1;
    height: 5px;
    min-width: 24px;
    margin-top: 5px; /* optical-centers the bar on the arrow's first line now that .vocab-row is flex-start (wrapped arrows need top alignment, not centering) */
    background: var(--border);
    border-radius: 3px;
    overflow: hidden;
  }
  .vocab-bar-fill {
    display: block;
    height: 100%;
    background: var(--accent-dim);
    border-radius: 3px;
  }
  .vocab-uses {
    flex: 0 0 auto;
    min-width: 52px;
    text-align: right;
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }
  .vocab-files {
    flex: 0 0 auto;
    min-width: 64px;
    text-align: right;
    color: var(--text-hint);
    font-size: 0.7rem;
  }
</style>
