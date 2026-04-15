<script>
  import { workbenchStore } from '../stores/workbench.js'

  export let node
  export let arrowDefs = []

  $: collectionDir = $workbenchStore.collectionDir ?? ''

  // Map ArrowDef.Code → LongName for display
  $: arrowDefMap = arrowDefs.reduce((m, d) => { m[d.Code] = d.LongName; return m }, {})

  // ALL CAPS reminder: text is all uppercase + length ≥ 3
  $: isReminder = /^[A-Z][A-Z0-9 _,;:.!?'"()[\]-]{2,}$/.test((node.Text ?? '').trim())

  const IMAGE_EXT = /\.(png|jpe?g|gif|webp|svg)$/i
  const AUDIO_EXT = /\.(mp3|wav|ogg)$/i

  function mediaTokens(text) {
    return (text ?? '').split(/\s+/).filter(t => IMAGE_EXT.test(t) || AUDIO_EXT.test(t))
  }

  function resolveMedia(token) {
    if (/^https?:\/\//.test(token)) return token
    return collectionDir ? `${collectionDir}/${token}` : token
  }
</script>

<div class="node-card" class:reminder={isReminder} class:sequence-node={node.IsSequence}>
  <p class="node-text">{node.Text}</p>

  <!-- Inline media -->
  {#each mediaTokens(node.Text) as token}
    {#if IMAGE_EXT.test(token)}
      <img class="inline-media" src={resolveMedia(token)} alt={token} loading="lazy" />
    {:else if AUDIO_EXT.test(token)}
      <!-- svelte-ignore a11y-media-has-caption -->
      <audio class="inline-audio" controls src={resolveMedia(token)} />
    {/if}
  {/each}

  <!-- Arrow properties -->
  {#if node.Arrows?.length || node.Dittos?.some(d => d.Arrows?.length)}
    <dl class="arrow-list">
      {#each node.Arrows ?? [] as arrow}
        <div class="arrow-entry">
          <dt class="arrow-dt">{arrowDefMap[arrow.Code] ?? arrow.Code}</dt>
          <dd class="arrow-dd">{arrow.Target}</dd>
        </div>
      {/each}
      {#each node.Dittos ?? [] as ditto}
        {#if ditto.Text}
          <div class="arrow-entry ditto">
            <dd class="arrow-dd ditto-text">{ditto.Text}</dd>
          </div>
        {/if}
        {#each ditto.Arrows ?? [] as arrow}
          <div class="arrow-entry ditto">
            <dt class="arrow-dt">{arrowDefMap[arrow.Code] ?? arrow.Code}</dt>
            <dd class="arrow-dd">{arrow.Target}</dd>
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

  .inline-media {
    display: block;
    max-width: 100%;
    border-radius: 4px;
    margin: 6px 0;
  }

  .inline-audio {
    display: block;
    width: 100%;
    margin: 6px 0;
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
