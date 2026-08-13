<script>
  // Right-side slide-in reference for the graph query language. Content is
  // generated entirely from lib/query-grammar.js (DIRECTIVES / MATCHING /
  // COMBINING) so the docs and the autocomplete can never drift. Opened by
  // either query bar's "?" via the shared queryHelpOpen store.
  //
  // Dismiss: the X button, clicking the overlay backdrop, or Esc.

  import { queryHelpOpen } from '../stores/queryHints.js'
  import { DIRECTIVES, MATCHING, COMBINING } from '../lib/query-grammar.js'

  function close() {
    queryHelpOpen.set(false)
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && $queryHelpOpen) {
      e.preventDefault()
      e.stopPropagation()
      close()
    }
  }
</script>

<svelte:window on:keydown|capture={onKeydown} />

{#if $queryHelpOpen}
  <!-- Non-modal: no click-catching backdrop, so focusing/clicking a query
       bar (or anywhere else) leaves the panel open. Dismiss via the X button
       or Esc. -->
  <div class="help-panel" role="dialog" aria-label="Query syntax help">
    <div class="help-header">
      <span class="help-title">Graph query syntax</span>
      <button class="close-btn" type="button" title="Close (Esc)" aria-label="Close" on:click={close}>✕</button>
    </div>

    <div class="help-body">
      <section>
        <h3>Directives</h3>
        <p class="lead">{COMBINING.rule}</p>
        <table class="dir-table">
          <tbody>
            {#each DIRECTIVES as d}
              <tr>
                <td class="cell-token">
                  <code>{d.token}{d.arg ? ' ' + d.arg : ''}</code>
                  {#if d.aliases.length}
                    <div class="aliases">aka {d.aliases.join(', ')}</div>
                  {/if}
                </td>
                <td class="cell-summary">
                  {d.summary}
                  <div class="example"><code>{d.example}</code></div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </section>

      <section>
        <h3>Combining directives</h3>
        <div class="combine">
          <code class="combine-q">{COMBINING.example}</code>
          <p>{COMBINING.explanation}</p>
        </div>
      </section>

      <section>
        <h3>Matching words</h3>
        <ul class="match-list">
          {#each MATCHING as m}
            <li>
              <span class="match-title">{m.title}</span>
              <span class="match-body">{m.body}</span>
            </li>
          {/each}
        </ul>
      </section>
    </div>
  </div>
{/if}

<style>
  .help-panel {
    position: fixed;
    top: 0;
    right: 0;
    bottom: 0;
    z-index: 211;
    width: min(460px, 92vw);
    background: var(--surface-overlay);
    border-left: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    box-shadow: -4px 0 16px rgba(0, 0, 0, 0.4);
  }

  .help-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .help-title {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--accent-dim);
    cursor: pointer;
    font-size: 0.95rem;
    padding: 2px 6px;
  }
  .close-btn:hover { color: var(--text); }

  .help-body {
    flex: 1;
    overflow-y: auto;
    padding: 12px 14px;
    color: var(--help-body-text);
    font-size: 0.85rem;
    line-height: 1.45;
  }

  section { margin-bottom: 20px; }

  h3 {
    margin: 0 0 8px;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--accent-dim);
  }

  .lead { margin: 0 0 10px; color: var(--help-lead-text); }

  code {
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: #67e8f9;
  }

  .dir-table { width: 100%; border-collapse: collapse; }
  .dir-table td {
    vertical-align: top;
    padding: 7px 6px;
    border-top: 1px solid var(--help-divider);
  }
  .cell-token { width: 42%; }
  .cell-token code { white-space: nowrap; }
  .aliases { margin-top: 3px; font-size: 0.72rem; color: var(--accent-dim); }
  .cell-summary { color: var(--text-block); }
  .example { margin-top: 4px; }
  .example code { color: #8fd9a8; font-size: 0.8rem; }

  .combine {
    background: var(--surface-block);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 12px;
  }
  .combine-q { display: block; margin-bottom: 6px; color: #8fd9a8; }
  .combine p { margin: 0; color: var(--text-block); }

  .match-list { list-style: none; margin: 0; padding: 0; }
  .match-list li { padding: 6px 0; border-top: 1px solid var(--help-divider); }
  .match-list li:first-child { border-top: 0; }
  .match-title {
    display: block;
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    color: #67e8f9;
    margin-bottom: 2px;
  }
  .match-body { color: var(--text-block); }
</style>
