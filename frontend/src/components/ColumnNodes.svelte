<script>
  import { createEventDispatcher } from 'svelte'
  import { workbenchStore, filteredNodes } from '../stores/workbench.js'
  import { hasDetail, NODE_FILTERS } from '../lib/node-filters.js'

  const dispatch = createEventDispatcher()

  // ── Local state ──────────────────────────────────────────────────────────────
  let sortAlpha = false
  let quickFilter = 'all'           // one of NODE_FILTERS[].id
  let expanded = new Set()          // Set<number> of expanded node.Line values
  let nodeListEl = null             // bind:this on the scroll container

  // ── Helpers ──────────────────────────────────────────────────────────────────

  // Flatten all displayable ditto rows: each ditto contributes text (if any) + arrow (if any).
  // Rows from the root node's own Arrows are included first (no text, arrow only).
  function allDittoRows(node) {
    const rows = []
    const rootArrows = node.Arrows ?? []
    for (let i = 0; i < rootArrows.length; i++) {
      const a = rootArrows[i]
      rows.push({ key: `root_${a.Line}_${a.Code}`, text: '', arrow: a, depth: i })
    }
    for (const d of (node.Dittos ?? [])) {
      const text = d.Text ?? ''
      const arrows = d.Arrows ?? []
      if (arrows.length === 0) {
        if (text) rows.push({ key: `d_${d.Line}`, text, arrow: null, depth: 0 })
      } else {
        rows.push({ key: `d_${d.Line}_${arrows[0].Code}`, text, arrow: arrows[0], depth: 0 })
        for (let i = 1; i < arrows.length; i++) {
          rows.push({ key: `d_${d.Line}_${arrows[i].Code}_${i}`, text: '', arrow: arrows[i], depth: i })
        }
      }
    }
    return rows
  }

  function toggleExpand(line) {
    expanded = new Set(expanded)    // new reference → Svelte reactivity
    expanded.has(line) ? expanded.delete(line) : expanded.add(line)
  }

  function selectNode(node) {
    dispatch('node-selected', node.Line)
  }

  // ── Derived display data ──────────────────────────────────────────────────────

  $: arrowDefMap = $workbenchStore.arrowDefs
      .reduce((m, d) => { m[d.Code] = d.LongName; return m }, {})

  $: selectedLine = $workbenchStore.selectedNodeLine

  // Quick filter stacks on top of the context-filtered derived store. The
  // table's 'all' entry matches everything, so there is no special case here.
  $: activeFilter = NODE_FILTERS.find((f) => f.id === quickFilter) ?? NODE_FILTERS[0]
  $: quickFiltered = $filteredNodes.filter(activeFilter.match)

  // A–Z sort
  $: displayNodes = sortAlpha
      ? [...quickFiltered].sort((a, b) => a.Text.localeCompare(b.Text))
      : quickFiltered

  // Sequence numbering: {node.Line → 1-based index} among IsSequence nodes in display order
  $: seqNumMap = (() => {
    const m = {}; let i = 1
    for (const n of displayNodes) if (n.IsSequence) m[n.Line] = i++
    return m
  })()

  // Editor cursor → highlight the node whose Line is the largest ≤ editorCursorLine.
  // Searches ALL chapter nodes (not just filtered) so the cursor is always tracked.
  $: isParsing = $workbenchStore.isParsing
  $: editorCursorLine = $workbenchStore.editorCursorLine
  $: cursorMatchedLine = (() => {
    if (!editorCursorLine) return null
    const ch = $workbenchStore.parsedFile?.Chapters?.find(
      (c) => c.Title === $workbenchStore.selectedChapterKey?.chapterTitle
    )
    if (!ch) return null
    let match = null
    for (const n of (ch.Nodes ?? []))
      if (n.Line <= editorCursorLine) match = n.Line
    return match
  })()

  // Scroll Column 3 to keep the cursor-tracked node visible.
  // Debounced so rapid key-repeat doesn't cause jitter; block:'nearest' means
  // no scroll occurs when the node is already fully in view.
  let _cursorScrollTimer = null
  $: if (cursorMatchedLine && nodeListEl) {
    clearTimeout(_cursorScrollTimer)
    _cursorScrollTimer = setTimeout(() => {
      const el = nodeListEl.querySelector(`[data-line="${cursorMatchedLine}"]`)
      el?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }, 120)
  }

  // Direct scroll when a node is selected via omnibar navigation.
  // Uses a dedicated store field so there is no race with SourceTab clearing
  // selectedNodeLine. We own this field: read it, scroll, then clear it.
  $: if ($workbenchStore.nodeListScrollTarget) {
    const line = $workbenchStore.nodeListScrollTarget
    workbenchStore.update(s => ({ ...s, nodeListScrollTarget: null }))
    requestAnimationFrame(() => {
      nodeListEl?.querySelector(`[data-line="${line}"]`)
        ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    })
  }
</script>

<div class="col-nodes">
  <!-- Header row: title + optional spinner + A–Z -->
  <div class="col-header">
    <span class="col-title">Nodes</span>
    {#if isParsing}
      <span class="parse-spinner" title="Parsing…" />
    {/if}
    <button
      class="sort-btn"
      class:active={sortAlpha}
      on:click={() => (sortAlpha = !sortAlpha)}
      title="Sort alphabetically"
    >A–Z</button>
  </div>

  <!-- Quick filter bar -->
  <div class="quick-filter-bar">
    {#each NODE_FILTERS as f (f.id)}
      <button class="qf-btn" class:active={quickFilter === f.id} aria-pressed={quickFilter === f.id} on:click={() => (quickFilter = f.id)}>{f.label}</button>
    {/each}
  </div>

  <!-- Node list -->
  <div class="node-list" class:parsing={isParsing} bind:this={nodeListEl}>
    {#if displayNodes.length === 0}
      <div class="empty-hint">No nodes</div>
    {:else}
      {#each displayNodes as node (node.Line)}

        {#if node.IsSequence}
          <!-- ── C: Sequence timeline item ─────────────────────────────────── -->
          <div class="timeline-item"
               data-line={node.Line}
               class:active={selectedLine === node.Line}
               class:cursor-at={cursorMatchedLine === node.Line}>
            <div class="timeline-left">
              <span class="seq-num">({seqNumMap[node.Line]})</span>
              <div class="timeline-stem" />
            </div>
            <div class="timeline-content">
              {#if hasDetail(node)}
                <div class="node-row">
                  <button class="chevron-btn"
                          on:click={() => toggleExpand(node.Line)}
                          aria-label={expanded.has(node.Line) ? 'Collapse' : 'Expand'}>
                    {expanded.has(node.Line) ? '▼' : '▶'}
                  </button>
                  <button class="node-item" on:click={() => selectNode(node)}>
                    <span class="node-text">{node.Text}</span>
                  </button>
                  <div class="hover-actions">
                    <button class="ha-btn" on:click|stopPropagation={() => console.log('Promote to Card', node.Line)}>+ Card</button>
                  </div>
                </div>
                {#if expanded.has(node.Line)}
                  <div class="arrow-children">
                    {#each allDittoRows(node) as row (row.key)}
                      <div class="arrow-child"
                           class:unregistered={row.arrow && !row.arrow.IsRegistered}
                           style={row.depth > 0 ? `padding-left: ${row.depth * 20}px` : ''}>
                        {#if row.text}
                          <span class="ditto-text">{row.text}</span>
                        {/if}
                        {#if row.arrow}
                          <span class="arrow-code">{arrowDefMap[row.arrow.Code] ?? row.arrow.Code}</span>
                          <span class="arrow-sep">→</span>
                          <span class="arrow-target">{row.arrow.Target}</span>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/if}
              {:else}
                <div class="node-row">
                  <button class="node-item extract-node" on:click={() => selectNode(node)}>
                    <span class="extract-icon" aria-hidden="true">"</span>
                    <span class="node-text">{node.Text}</span>
                  </button>
                  <div class="hover-actions">
                    <button class="ha-btn" on:click|stopPropagation={() => console.log('Promote to Card', node.Line)}>+ Card</button>
                  </div>
                </div>
              {/if}
            </div>
          </div>

        {:else if hasDetail(node)}
          <!-- ── B: Structured / collapsible ───────────────────────────────── -->
          <div class="node-row-wrap"
               data-line={node.Line}
               class:active={selectedLine === node.Line}
               class:cursor-at={cursorMatchedLine === node.Line}>
            <div class="node-row">
              <button class="chevron-btn"
                      on:click={() => toggleExpand(node.Line)}
                      aria-label={expanded.has(node.Line) ? 'Collapse' : 'Expand'}>
                {expanded.has(node.Line) ? '▼' : '▶'}
              </button>
              <button class="node-item" on:click={() => selectNode(node)}>
                <span class="node-text">{node.Text}</span>
              </button>
              <div class="hover-actions">
                <button class="ha-btn" on:click|stopPropagation={() => console.log('Promote to Card', node.Line)}>+ Card</button>
              </div>
            </div>
            {#if expanded.has(node.Line)}
              <div class="arrow-children">
                {#each allDittoRows(node) as row (row.key)}
                  <div class="arrow-child"
                       class:unregistered={row.arrow && !row.arrow.IsRegistered}
                       style={row.depth > 0 ? `padding-left: ${row.depth * 20}px` : ''}>
                    {#if row.text}
                      <span class="ditto-text">{row.text}</span>
                    {/if}
                    {#if row.arrow}
                      <span class="arrow-code">{arrowDefMap[row.arrow.Code] ?? row.arrow.Code}</span>
                      <span class="arrow-sep">→</span>
                      <span class="arrow-target">{row.arrow.Target}</span>
                    {/if}
                  </div>
                {/each}
              </div>
            {/if}
          </div>

        {:else}
          <!-- ── A: Extract / pure text ─────────────────────────────────────── -->
          <div class="node-row-wrap"
               data-line={node.Line}
               class:active={selectedLine === node.Line}
               class:cursor-at={cursorMatchedLine === node.Line}>
            <div class="node-row">
              <button class="node-item extract-node" on:click={() => selectNode(node)}>
                <span class="extract-icon" aria-hidden="true">"</span>
                <span class="node-text">{node.Text}</span>
              </button>
              <div class="hover-actions">
                <button class="ha-btn" on:click|stopPropagation={() => console.log('Promote to Card', node.Line)}>+ Card</button>
              </div>
            </div>
          </div>
        {/if}

      {/each}
    {/if}
  </div>
</div>

<style>
  /* ── Layout ──────────────────────────────────────────────────────────────── */
  .col-nodes {
    /* width is owned by the parent slot in TopBand (flex remainder) */
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border-right: 1px solid var(--border);
  }

  .col-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 4px 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .col-title {
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--accent-dim);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  /* ── Parse spinner ──────────────────────────────────────────────────────── */
  .parse-spinner {
    display: inline-block;
    width: 10px;
    height: 10px;
    border: 2px solid var(--border);
    border-top-color: var(--parse-spinner-accent);
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
    flex-shrink: 0;
    margin-right: auto;   /* push A–Z to the right */
    margin-left: 6px;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .node-list.parsing {
    opacity: 0.4;
    pointer-events: none;
  }

  .sort-btn {
    padding: 2px 6px;
    background: none;
    border: 1px solid var(--border);
    border-radius: 3px;
    color: var(--accent-dim);
    font-size: 0.7rem;
    cursor: pointer;
  }
  .sort-btn.active { border-color: var(--accent-primary); color: var(--accent-text-alt); }

  /* ── Quick filter bar ────────────────────────────────────────────────────── */
  .quick-filter-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 3px;
    padding: 3px 8px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
    background: var(--surface-bar);
  }

  .qf-btn {
    padding: 2px 8px;
    background: none;
    border: 1px solid var(--border);
    border-radius: 3px;
    color: var(--accent-dim);
    font-size: 0.7rem;
    cursor: pointer;
    transition: color 0.1s, border-color 0.1s;
  }
  .qf-btn:hover   { color: var(--accent-text-alt); border-color: var(--control-border-hover); }
  .qf-btn.active  { border-color: var(--accent-primary); color: var(--accent-text-alt); background: var(--control-active); }

  /* ── Node list scroll container ──────────────────────────────────────────── */
  .node-list {
    flex: 1;
    overflow-y: auto;
    padding: 2px 0;
  }

  .empty-hint {
    padding: 12px 8px;
    color: var(--text-dim);
    font-size: 0.8rem;
  }

  /* ── Shared row wrapper (B + A) ───────────────────────────────────────────── */
  .node-row-wrap {
    position: relative;
  }
  .node-row-wrap.active   { background: var(--accent); }
  .node-row-wrap.cursor-at:not(.active) { background: var(--row-cursor-bg); }

  /* ── Node row (chevron + label + hover actions) ───────────────────────────── */
  .node-row {
    display: flex;
    align-items: center;
    position: relative;
    min-height: 28px;
  }

  /* ── Chevron button ───────────────────────────────────────────────────────── */
  .chevron-btn {
    flex-shrink: 0;
    width: 20px;
    padding: 0;
    background: none;
    border: none;
    color: var(--chevron-muted);
    font-size: 0.65rem;
    cursor: pointer;
    line-height: 1;
    text-align: center;
  }
  .chevron-btn:hover { color: var(--accent-text-alt); }

  /* ── Node item (the clickable text button) ────────────────────────────────── */
  .node-item {
    flex: 1;
    min-width: 0;
    padding: 5px 4px 5px 0;
    background: none;
    border: none;
    color: var(--text-row);
    font-size: 0.82rem;
    text-align: left;
    cursor: pointer;
    display: flex;
    align-items: baseline;
    gap: 4px;
    overflow: hidden;
  }
  .node-item:hover { color: var(--text); }
  .node-row-wrap.active .node-item { color: var(--text); }

  /* .arrow-children lives inside .node-row-wrap, so when the row is active
     (background var(--accent)) or cursor-matched (var(--row-cursor-bg)) its
     arrow-code/arrow-target text sits on that fill instead of the plain
     list surface --arrow-code-text and var(--n4l-syntax-arrow-target) were
     solved against (Task 5 review round 1 — Important 2: Light measured
     3.96:1/4.07:1 there, Dark 4.31:1 where the unconverted literal had been
     9.83:1, an outright regression). Same fix as .node-item above rather
     than re-solving the shared tokens themselves — var(--n4l-syntax-arrow-
     target) in particular is used elsewhere (NodeCard, the editor grammar)
     and re-tuning it for this one row state would ripple into those.
     var(--text) clears both backgrounds in every theme (9.64-14.58:1). */
  .node-row-wrap.active .arrow-code,
  .node-row-wrap.active .arrow-target,
  .node-row-wrap.cursor-at:not(.active) .arrow-code,
  .node-row-wrap.cursor-at:not(.active) .arrow-target {
    color: var(--text);
  }

  /* Extract: left-pad to align with chevron'd items */
  .node-item.extract-node { padding-left: 4px; }

  .node-text {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* Extract icon: muted quotation mark */
  .extract-icon {
    flex-shrink: 0;
    color: var(--accent-faint);
    font-size: 0.95rem;
    line-height: 1;
    font-family: Georgia, serif;
  }

  /* ── Hover action buttons ─────────────────────────────────────────────────── */
  .hover-actions {
    position: absolute;
    right: 4px;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    gap: 3px;
    visibility: hidden;
    pointer-events: none;
  }
  .node-row:hover .hover-actions {
    visibility: visible;
    pointer-events: auto;
  }

  .ha-btn {
    padding: 2px 6px;
    background: var(--control-active);
    border: 1px solid var(--control-border);
    border-radius: 3px;
    color: var(--ha-btn-text);
    font-size: 0.7rem;
    cursor: pointer;
  }
  .ha-btn:hover { background: var(--accent); color: var(--text); }

  /* ── Arrow children (expanded tree) ──────────────────────────────────────── */
  .arrow-children {
    padding: 2px 0 4px 28px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .arrow-child {
    display: flex;
    align-items: baseline;
    gap: 5px;
    font-size: 0.78rem;
    flex-wrap: wrap;
  }

  .arrow-code {
    color: var(--arrow-code-text);
    font-weight: 600;
    white-space: nowrap;
    flex-shrink: 0;
  }
  .arrow-child.unregistered .arrow-code { color: var(--warn); }

  .arrow-sep {
    color: var(--arrow-sep);
    flex-shrink: 0;
  }

  .arrow-target {
    color: var(--n4l-syntax-arrow-target);
    overflow-wrap: anywhere;
  }

  .ditto-text {
    color: var(--text-muted);
    font-style: italic;
    overflow-wrap: anywhere;
  }

  /* ── Sequence timeline ────────────────────────────────────────────────────── */
  .timeline-item {
    display: flex;
    position: relative;
  }
  .timeline-item.active   { background: var(--accent); }
  .timeline-item.cursor-at:not(.active) { background: var(--row-cursor-bg); }

  .timeline-left {
    display: flex;
    flex-direction: column;
    align-items: center;
    flex-shrink: 0;
    width: 28px;
    padding-top: 6px;
  }

  .seq-num {
    font-size: 0.65rem;
    color: var(--seq-num);
    font-weight: 700;
    line-height: 1;
    white-space: nowrap;
  }

  .timeline-stem {
    width: 2px;
    flex: 1;
    background: var(--seq-stem);
    margin-top: 2px;
    min-height: 8px;
  }
  /* Hide stem on the last timeline item */
  .timeline-item:last-child .timeline-stem { visibility: hidden; }
  /* Also hide when next sibling is not a timeline item */
  .timeline-item:not(:has(+ .timeline-item)) .timeline-stem { visibility: hidden; }

  .timeline-content {
    flex: 1;
    min-width: 0;
  }
</style>
