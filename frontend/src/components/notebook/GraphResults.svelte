<script>
  export let result = null
  $: r = result ?? {}
  $: empty = r && !r.GraphUnavailable && !r.GraphError &&
    !(r.Nodes?.length || r.Structural?.length || r.Listings?.length || r.Notes?.length)
</script>

<div class="gr">
  {#if r.GraphUnavailable}
    <p class="gr-hint">Graph not built — run Build Graph (Ctrl+G) first.</p>
  {:else if r.GraphError}
    <p class="gr-error">{r.GraphError}</p>
  {:else if empty}
    <p class="gr-hint">No results.</p>
  {:else}
    {#if r.Nodes?.length}
      <section>
        {#each r.Nodes as n}
          <div class="node">
            <div class="node-head">{n.NodeText}{#if n.Chapter}<span class="chap">· {n.Chapter}</span>{/if}</div>
            {#if n.ContextTags?.length}<div class="tags">{#each n.ContextTags as t}<span class="tag">{t}</span>{/each}</div>{/if}
            {#if n.Orbit?.length}
              <ul class="orbit">
                {#each n.Orbit as o}<li><span class="arrow">{o.ArrowLong}</span> {o.NeighborText}{#if o.Radius > 1}<span class="rad"> r{o.Radius}</span>{/if}</li>{/each}
              </ul>
            {/if}
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Structural?.length}
      <section class="chains">
        {#each r.Structural as chain}
          <div class="chain" class:seq={chain.Kind === 'sequence'}>
            {#if chain.Context}<div class="chain-label">{chain.Context}{#if chain.Chapter}<span class="chap"> · {chain.Chapter}</span>{/if}</div>{/if}
            <div class="chain-nodes">
              {#each chain.Nodes as cn, i}
                <span class="cn">{cn.NodeText}</span>{#if cn.ArrowLong}<span class="cn-arrow"> —{cn.ArrowLong}→ </span>{/if}
              {/each}
            </div>
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Listings?.length}
      <section class="listings">
        {#each r.Listings as it}
          <div class="listing"><b>{it.Title}</b>{#if it.Detail}<span class="detail"> — {it.Detail}</span>{/if}
            {#if it.Nodes?.length}<div class="listing-nodes">{it.Nodes.join(', ')}</div>{/if}
          </div>
        {/each}
      </section>
    {/if}

    {#if r.Notes?.length}
      <section class="notes">
        {#each r.Notes as note}
          <div class="note-chap">{note.Chapter}</div>
          {#each note.Blocks as b}
            <div class="nb"><div class="nb-head">{b.NodeText}{#if b.Context}<span class="chap"> · {b.Context}</span>{/if}</div>
              {#if b.Arrows?.length}<ul class="orbit">{#each b.Arrows as a}<li><span class="arrow">{a.ArrowLong}</span> {a.Target}</li>{/each}</ul>{/if}
            </div>
          {/each}
        {/each}
      </section>
    {/if}
  {/if}
</div>

<style>
  .gr { font-size: 12px; color: #cdd6e2; display: flex; flex-direction: column; gap: 10px; }
  .gr-hint { color: #5a7a9a; margin: 0; }
  .gr-error { color: #ff8080; margin: 0; }
  section { display: flex; flex-direction: column; gap: 6px; }
  .node, .nb { background: #101826; border: 1px solid #26374a; border-radius: 6px; padding: 6px 8px; }
  .node-head, .nb-head { color: #e8eaf0; font-weight: 600; }
  .chap { color: #5a7a9a; font-weight: 400; font-size: 11px; }
  .tags { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 3px; }
  .tag { background: #1e3a5a; color: #93c5fd; border-radius: 4px; padding: 0 5px; font-size: 11px; }
  .orbit { margin: 4px 0 0; padding-left: 14px; color: #b9c6d6; }
  .arrow { color: #7dd3fc; }
  .rad { color: #5a7a9a; font-size: 10px; }
  .chain { background: #101826; border: 1px solid #26374a; border-radius: 6px; padding: 6px 8px; }
  .chain-label { color: #93c5fd; font-size: 11px; margin-bottom: 3px; }
  .cn { color: #e8eaf0; }
  .cn-arrow { color: #7dd3fc; }
  .chain.seq .chain-nodes { display: flex; flex-direction: column; }
  .listing { padding: 2px 0; }
  .detail { color: #b9c6d6; }
  .listing-nodes { color: #8aa0b6; font-size: 11px; }
  .note-chap { color: #7dd3fc; font-size: 11px; margin-top: 4px; }
</style>
