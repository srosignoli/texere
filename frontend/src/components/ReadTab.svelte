<script>
  import { workbenchStore } from '../stores/workbench.js'
  import NodeCard from './NodeCard.svelte'

  $: arrowDefs = $workbenchStore.arrowDefs
  $: parsedFile = $workbenchStore.parsedFile
  $: selectedKey = $workbenchStore.selectedChapterKey

  $: chapter = parsedFile?.Chapters?.find(
    (ch) => ch.Title === selectedKey?.chapterTitle
  ) ?? null

  $: nodes = chapter?.Nodes ?? []

  // Group consecutive sequence nodes into runs for connector rendering
  function groupNodes(nodeList) {
    const groups = []
    let i = 0
    while (i < nodeList.length) {
      if (nodeList[i].IsSequence) {
        const run = [nodeList[i]]
        while (i + 1 < nodeList.length && nodeList[i + 1].IsSequence) {
          i++
          run.push(nodeList[i])
        }
        groups.push({ kind: 'sequence', nodes: run })
      } else {
        groups.push({ kind: 'single', nodes: [nodeList[i]] })
      }
      i++
    }
    return groups
  }

  $: nodeGroups = groupNodes(nodes)
</script>

<div class="read-tab">
  {#if !selectedKey}
    <div class="empty-hint">Select a chapter to read</div>
  {:else if nodes.length === 0}
    <div class="empty-hint">No nodes in this chapter</div>
  {:else}
    <div class="node-feed">
      {#each nodeGroups as group}
        {#if group.kind === 'sequence'}
          <div class="sequence-group">
            {#each group.nodes as node (node.Line)}
              <NodeCard {node} {arrowDefs} />
            {/each}
          </div>
        {:else}
          {#each group.nodes as node (node.Line)}
            <NodeCard {node} {arrowDefs} />
          {/each}
        {/if}
      {/each}
    </div>
  {/if}
</div>

<style>
  .read-tab {
    height: 100%;
    overflow-y: auto;
    padding: 12px 16px;
    background: var(--surface-bar);
  }

  .empty-hint {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: var(--text-dim);
    font-size: 0.85rem;
  }

  .node-feed {
    max-width: 720px;
    margin: 0 auto;
  }

  /* Sequence group: vertical connector line on the left */
  .sequence-group {
    position: relative;
    padding-left: 16px;
    margin-bottom: 8px;
  }

  .sequence-group::before {
    content: '';
    position: absolute;
    left: 6px;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--sequence-connector);
    border-radius: 1px;
  }

  /* Amber reminder override — applied via NodeCard .reminder class */
  :global(.node-card.reminder) {
    background: var(--reminder-card-bg);
    border-left: 3px solid var(--warn);
  }

  /* Sequence connector between adjacent sequence-node cards */
  :global(.sequence-group .node-card.sequence-node + .node-card.sequence-node::before) {
    content: '';
    display: block;
    height: 8px;
    border-left: 2px solid var(--sequence-connector);
    margin-left: -14px;
    margin-bottom: -8px;
  }
</style>
