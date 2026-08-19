<script>
  import InlineMedia from './InlineMedia.svelte'
  import MathText from './MathText.svelte'
  import { mediaTokens } from '../lib/n4l-media.js'

  export let node
  export let arrowDefs = []

  // Map ArrowDef.Code → LongName for display
  $: arrowDefMap = arrowDefs.reduce((m, d) => { m[d.Code] = d.LongName; return m }, {})

  // ALL CAPS reminder: text is all uppercase + length ≥ 3
  $: isReminder = /^[A-Z][A-Z0-9 _,;:.!?'"()[\]-]{2,}$/.test((node.Text ?? '').trim())
</script>

<div class="node-card" class:reminder={isReminder} class:sequence-node={node.IsSequence}>
  <p class="node-text"><MathText text={node.Text} /></p>

  <!-- Media named in the node's own text -->
  {#each mediaTokens(node.Text) as token}
    <InlineMedia path={token} />
  {/each}

  <!-- Arrow properties -->
  {#if node.Arrows?.length || node.Dittos?.some(d => d.Arrows?.length)}
    <dl class="arrow-list">
      {#each node.Arrows ?? [] as arrow}
        <div class="arrow-entry">
          <dt class="arrow-dt">{arrowDefMap[arrow.Code] ?? arrow.Code}</dt>
          <dd class="arrow-dd"><MathText text={arrow.Target} /><InlineMedia path={arrow.Target} /></dd>
        </div>
      {/each}
      {#each node.Dittos ?? [] as ditto}
        {#if ditto.Text}
          <div class="arrow-entry ditto">
            <dd class="arrow-dd ditto-text"><MathText text={ditto.Text} /></dd>
          </div>
        {/if}
        {#each ditto.Arrows ?? [] as arrow}
          <div class="arrow-entry ditto">
            <dt class="arrow-dt">{arrowDefMap[arrow.Code] ?? arrow.Code}</dt>
            <dd class="arrow-dd"><MathText text={arrow.Target} /><InlineMedia path={arrow.Target} /></dd>
          </div>
        {/each}
      {/each}
    </dl>
  {/if}
</div>

<style>
  .node-card {
    background: var(--surface-raised);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 10px 14px;
    margin-bottom: 8px;
  }

  .node-card.reminder {
    background: var(--reminder-card-bg);
    border-left: 3px solid var(--warn);
    color: var(--reminder-card-text);
  }

  .node-card.sequence-node {
    border-left: 2px solid var(--sequence-connector);
  }

  .node-text {
    margin: 0 0 6px;
    font-size: 0.88rem;
    color: var(--text);
    line-height: 1.5;
  }

  .node-card.reminder .node-text {
    color: var(--reminder-card-text-strong);
    font-weight: 600;
  }

  .arrow-list {
    margin: 6px 0 0;
    padding: 0;
    border-top: 1px solid var(--border);
    padding-top: 6px;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .arrow-entry {
    display: flex;
    gap: 6px;
    font-size: 0.8rem;
  }

  .arrow-entry.ditto {
    opacity: 0.75;
  }

  .arrow-dt {
    color: var(--accent-dim);
    font-weight: 600;
    white-space: nowrap;
    min-width: 80px;
  }

  .arrow-dt::after {
    content: ':';
  }

  .arrow-dd {
    color: var(--n4l-syntax-arrow-target);
    margin: 0;
    overflow-wrap: anywhere;
  }

  /* .node-card.reminder freezes its own surface (--reminder-card-bg) to the
     same pale highlighter-yellow in every theme (Task 5, and reaffirmed on
     review round 1) — but --accent-dim, --n4l-syntax-arrow-target and
     --border, which .arrow-dt/.arrow-dd/.arrow-list normally use, are
     THEMED, and none of them were designed against a fixed pale-yellow
     floor: --n4l-syntax-arrow-target alone measures 1.26:1 in Blue and
     2.88:1 in Dark against --reminder-card-bg (both pre-existing, not
     introduced by this migration, but freezing the surface without also
     freezing what paints on it turns that pre-existing weakness permanent
     across every theme, not just Blue). Fixed by overriding the children
     inside a reminder card to the same frozen ink already used for
     .node-text, rather than re-theming the card (the sticky-note motif is a
     real argument for staying pale-in-every-theme; re-theming the surface
     back would abandon it, and would only move the mismatch onto whichever
     child colour DIDN'T get re-solved). */
  .node-card.reminder .arrow-dt {
    color: var(--reminder-card-text-strong);  /* 6.37:1 — same as .node-text, unchanged in every theme */
  }
  .node-card.reminder .arrow-dd {
    color: var(--reminder-card-text);  /* 16.70:1 — unchanged in every theme */
  }
  .node-card.reminder .arrow-list {
    border-top-color: var(--reminder-card-border);  /* ~3:1 non-text minimum, unchanged in every theme */
  }
</style>
