<script>
  import { AVAILABLE_THEMES } from '../lib/themes.js'
  import { theme, applyTheme } from '../stores/theme.js'
  import { llmSettingsOpen } from '../stores/workbench.js'

  let open = false
  let root // the wrapper, for the outside-click test
  let toggleEl

  function toggle() {
    open = !open
  }

  function close({ refocus = false } = {}) {
    if (!open) return
    open = false
    // Escape and outside-click differ: dismissing with the keyboard must not
    // drop focus onto <body>, or the next Tab restarts from the top of the app.
    if (refocus) toggleEl?.focus()
  }

  function onKeydown(e) {
    if (e.key === 'Escape' && open) {
      e.stopPropagation()
      close({ refocus: true })
    }
  }

  // pointerdown rather than click: a click that starts inside the popover and
  // ends outside it (a drag off a label) should not count as "outside".
  function onPointerDown(e) {
    if (open && root && !root.contains(e.target)) close()
  }
</script>

<svelte:window on:keydown={onKeydown} on:pointerdown={onPointerDown} />

<div class="sm-root" bind:this={root}>
  <button
    class="sm-toggle"
    class:sm-open={open}
    bind:this={toggleEl}
    on:click={toggle}
    title="Settings"
    aria-expanded={open}
    aria-controls="sm-popover"
    aria-label="Settings"
  >⚙</button>

  {#if open}
    <!-- A disclosure (button + aria-expanded + aria-controls), not a dialog:
         role="dialog" announces as modal, and this popover is deliberately not
         — no focus trap, and the click that dismisses it still reaches the app.
         The fieldset's legend names the contents, so the wrapper needs no role
         of its own. -->
    <div class="sm-popover" id="sm-popover">
      <fieldset class="sm-section">
        <legend class="sm-legend">Appearance</legend>
        {#each AVAILABLE_THEMES as t (t.id)}
          <label class="sm-choice">
            <input
              type="radio"
              name="theme"
              value={t.id}
              checked={$theme === t.id}
              on:change={() => applyTheme(t.id)}
            />
            <span class="sm-choice-label">{t.label}</span>
          </label>
        {/each}
      </fieldset>

      <fieldset class="sm-section">
        <legend class="sm-legend">LLM, Speech &amp; Skills</legend>
        <button
          type="button"
          class="sm-launch"
          on:click={() => { llmSettingsOpen.set(true); close() }}
        >Configure…</button>
      </fieldset>
    </div>
  {/if}
</div>

<style>
  /* The popover is anchored to the button, so the wrapper is the containing
     block. It also defines "inside" for the outside-click test. */
  .sm-root {
    position: relative;
    flex-shrink: 0;
    display: flex;
    align-items: center;
  }

  /* Matches MiddleBand's .btn — this control sits in that row and would read as
     a foreign object otherwise. The values come from the same roles, so it
     follows the band in every theme rather than only in Blue. */
  .sm-toggle {
    padding: 4px 10px;
    background: var(--control-raised);
    color: var(--text-secondary);
    border: 1px solid var(--border-strong);
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    line-height: 1.4;
    white-space: nowrap;
  }

  .sm-toggle:hover {
    background: var(--control-raised-hover);
    color: var(--text);
  }

  /* While the popover is open the button is a pressed control, not a hover
     target — the fill alone would be ambiguous next to the row's other buttons,
     so the border carries it too. */
  .sm-toggle.sm-open {
    background: var(--control-active);
    border-color: var(--control-border-hover);
    color: var(--accent-text);
  }

  .sm-popover {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 60;
    min-width: 168px;
    padding: 8px;
    text-align: left;
    background: var(--surface-overlay);
    border: 1px solid var(--border);
    border-radius: 6px;
    /* A shadow, not a lighter fill: on Light the popover surface is white on a
       near-white band, and only the shadow plus the border separate them. */
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }

  /* fieldset/legend for the grouping semantics, with its chrome removed. */
  .sm-section {
    margin: 0;
    padding: 0;
    border: none;
  }

  .sm-legend {
    padding: 0 0 4px;
    font-size: 10px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-muted);
  }

  .sm-choice {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 5px 6px;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    color: var(--text-row);
  }

  .sm-choice:hover {
    background: var(--chip-hover);
    color: var(--text);
  }

  /* accent-color themes the native radio's fill in both light and dark webview
     schemes — styling the dot by hand would mean rebuilding the focus ring too. */
  .sm-choice input {
    margin: 0;
    accent-color: var(--accent-primary);
    cursor: pointer;
  }

  .sm-choice input:focus-visible {
    outline: 2px solid var(--accent-text);
    outline-offset: 1px;
  }

  .sm-choice-label {
    line-height: 1.4;
  }

  /* Matches .sm-choice's padding/font scale so this section doesn't read as
     bolted on, but it's a button (not a radio row) so it gets its own
     background/border, matching comparable controls elsewhere in the app. */
  .sm-launch {
    width: 100%;
    padding: 5px 6px;
    background: var(--control);
    color: var(--text);
    border: 1px solid var(--control-border);
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.8rem;
    line-height: 1.4;
    text-align: left;
  }

  .sm-launch:hover {
    background: var(--control-raised-hover);
  }
</style>
