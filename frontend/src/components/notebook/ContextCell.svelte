<script>
  export let context = { extractText: '', tags: [], orbit: [], source: { kind: 'none' } }
  $: src = context.source ?? { kind: 'none' }
</script>

<div class="ctx-cell">
  <div class="ctx-badge">CONTEXT</div>

  <section>
    <h4>Extract</h4>
    <p class="extract">{context.extractText || '—'}</p>
  </section>

  {#if context.tags?.length}
    <section>
      <h4>Tags</h4>
      <div class="tags">{#each context.tags as t}<span class="tag">{t}</span>{/each}</div>
    </section>
  {/if}

  <section>
    <h4>Neighbors</h4>
    {#if context.orbit?.length}
      <ul class="orbit">
        {#each context.orbit as o}<li><span class="arrow">{o.arrowLong}</span> {o.neighborText}</li>{/each}
      </ul>
    {:else}
      <p class="hint">No graph neighbors — build the graph (Ctrl+G) to see them.</p>
    {/if}
  </section>

  <section>
    <h4>Source</h4>
    {#if src.kind === 'pdf'}
      <p class="hint">PDF source — <a href={'#'} on:click|preventDefault>{src.anchor || 'open source'}</a> (page text extraction is a later slice).</p>
    {:else if src.passage}
      <p class="passage">…{src.passage}…</p>
    {:else if src.anchor}
      <p class="hint">From: {src.anchor}</p>
    {:else}
      <p class="hint">No linked source.</p>
    {/if}
  </section>
</div>

<style>
  .ctx-cell { position: relative; background: var(--surface-inset); border: 1px solid var(--border-card); border-radius: 8px; padding: 12px 14px 12px; margin-bottom: 10px; }
  .ctx-badge { position: absolute; top: 8px; right: 10px; font-size: 9px; letter-spacing: .1em; color: var(--accent-dim); }
  section { margin-bottom: 10px; }
  h4 { margin: 0 0 4px; font-size: 10px; letter-spacing: .08em; text-transform: uppercase; color: var(--accent-text); }
  .extract { margin: 0; color: var(--text); font-size: 13px; }
  .passage { margin: 0; color: var(--text-cell-muted); font-size: 12px; font-style: italic; }
  .hint { margin: 0; color: var(--accent-dim); font-size: 12px; }
  .tags { display: flex; flex-wrap: wrap; gap: 4px; }
  .tag { background: var(--accent); color: var(--accent-text-alt); border-radius: 4px; padding: 1px 6px; font-size: 11px; }
  .orbit { margin: 0; padding-left: 14px; color: var(--text-modal); font-size: 12px; }
  .arrow { color: var(--accent-text); }
</style>
