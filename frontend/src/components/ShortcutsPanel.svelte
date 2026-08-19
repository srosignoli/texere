<script>
  // Right-side slide-in list of every keyboard shortcut in the app. Content is
  // generated entirely from lib/shortcuts.js's SHORTCUT_GROUPS, so the docs and
  // the guard in shortcuts.test.js can never drift. Opened by either the "?"
  // button in MiddleBand.svelte or F1 (App.svelte's global keydown handler),
  // both of which drive the shared shortcutsOpen store.
  //
  // Modelled on QueryHelpPanel.svelte: same slide-in shell, same token set.
  //
  // Dismiss: the X button, or Esc.

  import { shortcutsOpen } from '../stores/workbench.js'
  import { SHORTCUT_GROUPS } from '../lib/shortcuts.js'

  function close() {
    shortcutsOpen.set(false)
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && $shortcutsOpen) {
      e.preventDefault()
      e.stopPropagation()
      close()
    }
  }
</script>

<svelte:window on:keydown|capture={onKeydown} />

{#if $shortcutsOpen}
  <div class="help-panel" role="dialog" aria-label="Keyboard shortcuts">
    <div class="help-header">
      <span class="help-title">Keyboard shortcuts</span>
      <button class="close-btn" type="button" title="Close (Esc)" aria-label="Close" on:click={close}>✕</button>
    </div>

    <div class="help-body">
      {#each SHORTCUT_GROUPS as group}
        <section>
          <h3>{group.title}</h3>
          <ul class="item-list">
            {#each group.items as item}
              <li class="item-row">
                <span class="item-keys">
                  {#each item.keys as key}
                    <kbd>{key}</kbd>
                  {/each}
                </span>
                <span class="item-label">{item.label}</span>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
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
    width: min(420px, 92vw);
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
  section:last-child { margin-bottom: 0; }

  h3 {
    margin: 0 0 8px;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--accent-dim);
  }

  .item-list { list-style: none; margin: 0; padding: 0; }

  .item-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 7px 0;
    border-top: 1px solid var(--help-divider);
  }
  .item-row:first-child { border-top: 0; }

  .item-keys {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-shrink: 0;
  }

  kbd {
    font-family: var(--mono, ui-monospace, SFMono-Regular, Menlo, monospace);
    font-size: 0.75rem;
    color: var(--help-code-text);
    background: var(--surface-block);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 1px 6px;
    line-height: 1.5;
  }

  .item-label {
    color: var(--text-block);
    text-align: right;
  }
</style>
