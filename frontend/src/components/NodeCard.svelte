<script>
  import InlineMedia from './InlineMedia.svelte'
  import MathText from './MathText.svelte'
  import { mediaKind } from '../lib/n4l-media.js'

  export let node
  export let arrowDefs = []

  // Map ArrowDef.Code → LongName for display
  $: arrowDefMap = arrowDefs.reduce((m, d) => { m[d.Code] = d.LongName; return m }, {})

  // ALL CAPS reminder: text is all uppercase + length ≥ 3
  $: isReminder = /^[A-Z][A-Z0-9 _,;:.!?'"()[\]-]{2,}$/.test((node.Text ?? '').trim())

  // Media referenced inside the node's own text, as opposed to as an arrow
  // target. mediaKind decides, so this view and the notes editor agree on the
  // extensions — the old local regexes knew nothing about video.
  function mediaTokens(text) {
    return (text ?? '').split(/\s+/).filter((t) => mediaKind(t))
  }
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
    background: #1e2f40;
    border: 1px solid #2a3a4a;
    border-radius: 6px;
    padding: 10px 14px;
    margin-bottom: 8px;
  }

  .node-card.reminder {
    background: #fef3c7;
    border-left: 3px solid #f59e0b;
    color: #1c1100;
  }

  .node-card.sequence-node {
    border-left: 2px solid #4b5563;
  }

  .node-text {
    margin: 0 0 6px;
    font-size: 0.88rem;
    color: #e8eaf0;
    line-height: 1.5;
  }

  .node-card.reminder .node-text {
    color: #92400e;
    font-weight: 600;
  }

  .arrow-list {
    margin: 6px 0 0;
    padding: 0;
    border-top: 1px solid #2a3a4a;
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
    color: #5a7a9a;
    font-weight: 600;
    white-space: nowrap;
    min-width: 80px;
  }

  .arrow-dt::after {
    content: ':';
  }

  .arrow-dd {
    color: #86efac;
    margin: 0;
    overflow-wrap: anywhere;
  }
</style>
