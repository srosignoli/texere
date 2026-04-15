<script>
  import { createEventDispatcher } from 'svelte'
  import ColumnChapters from './ColumnChapters.svelte'
  import ColumnContextTags from './ColumnContextTags.svelte'
  import ColumnNodes from './ColumnNodes.svelte'

  const dispatch = createEventDispatcher()

  function handleChapterSelected(e) {
    dispatch('chapter-selected', e.detail)
  }

  function handleNodeSelected(e) {
    dispatch('node-selected', e.detail)
  }
</script>

<div class="top-band">
  <ColumnChapters on:chapter-selected={handleChapterSelected} />
  <ColumnContextTags />
  <ColumnNodes on:node-selected={handleNodeSelected} />
</div>

<style>
  .top-band {
    display: flex;
    flex-direction: row;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }
</style>
