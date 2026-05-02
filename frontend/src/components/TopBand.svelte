<script>
  import { createEventDispatcher } from 'svelte'
  import ColumnChapters from './ColumnChapters.svelte'
  import ColumnContextTags from './ColumnContextTags.svelte'
  import ColumnNodes from './ColumnNodes.svelte'
  import Splitter from './Splitter.svelte'
  import { uiLayout } from '../stores/uiLayout.js'

  const dispatch = createEventDispatcher()

  function handleChapterSelected(e) {
    dispatch('chapter-selected', e.detail)
  }

  function handleNodeSelected(e) {
    dispatch('node-selected', e.detail)
  }

  function setChaptersWidth(e) {
    uiLayout.update(s => ({ ...s, chaptersWidth: e.detail }))
  }

  function setContextTagsWidth(e) {
    uiLayout.update(s => ({ ...s, contextTagsWidth: e.detail }))
  }
</script>

<div class="top-band">
  <div class="col-slot" style="width: {$uiLayout.chaptersWidth}px;">
    <ColumnChapters on:chapter-selected={handleChapterSelected} />
  </div>
  <Splitter
    orientation="vertical"
    size={$uiLayout.chaptersWidth}
    min={120}
    max={420}
    on:resize={setChaptersWidth}
  />
  <div class="col-slot" style="width: {$uiLayout.contextTagsWidth}px;">
    <ColumnContextTags />
  </div>
  <Splitter
    orientation="vertical"
    size={$uiLayout.contextTagsWidth}
    min={120}
    max={420}
    on:resize={setContextTagsWidth}
  />
  <div class="col-slot col-slot--flex">
    <ColumnNodes on:node-selected={handleNodeSelected} />
  </div>
</div>

<style>
  .top-band {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  /* Each column is wrapped in a fixed-width slot so the parent (TopBand)
     owns the sizing while the column components retain their own internal
     flex layout. The third slot flexes to absorb the remainder. */
  .col-slot {
    display: flex;
    flex: 0 0 auto;
    flex-direction: column;
    overflow: hidden;
    min-width: 0;
  }
  .col-slot--flex {
    flex: 1 1 0;
    width: auto;
  }

  /* Column components keep their internal styling but stop dictating the
     wrapper width — their root element stretches to fill the slot. */
  .col-slot > :global(.col-chapters),
  .col-slot > :global(.col-context-tags),
  .col-slot > :global(.col-nodes) {
    flex: 1 1 auto;
    width: 100%;
    height: 100%;
  }
</style>
