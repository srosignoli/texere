<script>
  import { workbenchStore } from '../stores/workbench.js'
  import SourceTab from './SourceTab.svelte'
  import ReadTab from './ReadTab.svelte'

  // Tab components: ReadTab added T041
  $: activeTab = $workbenchStore.activeTab

  // Reference to SourceTab for imperative calls from MiddleBand (T035)
  export let sourceTabRef = null

  function setTab(tab) {
    workbenchStore.update((s) => ({ ...s, activeTab: tab }))
  }

  function handleContentChanged() {
    workbenchStore.update((s) => ({ ...s, isDirty: true }))
  }
</script>

<div class="bottom-band">
  <div class="tab-bar">
    <button class="tab-btn" class:active={activeTab === 'note'} on:click={() => setTab('note')}>Note</button>
    <button class="tab-btn" class:active={activeTab === 'read'} on:click={() => setTab('read')}>Read</button>
  </div>

  <div class="tab-content">
    <!-- SourceTab always mounted so its EditorView is preserved; visibility toggled via CSS -->
    <div class="tab-pane" class:hidden={activeTab !== 'note'}>
      <SourceTab bind:this={sourceTabRef} on:content-changed={handleContentChanged} />
    </div>

    {#if activeTab === 'read'}
      <div class="tab-pane">
        <ReadTab />
      </div>
    {/if}
  </div>
</div>

<style>
  .bottom-band {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }

  .tab-bar {
    display: flex;
    flex-shrink: 0;
    background: #1a2535;
    border-bottom: 1px solid #2a3a4a;
    padding: 0 8px;
    gap: 2px;
  }

  .tab-btn {
    padding: 6px 16px;
    background: none;
    color: #8899aa;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-size: 0.85rem;
    transition: color 0.15s;
  }

  .tab-btn:hover {
    color: #c8d0dc;
  }

  .tab-btn.active {
    color: #e8eaf0;
    border-bottom-color: #2563eb;
  }

  .tab-content {
    flex: 1;
    overflow: hidden;
  }

  .tab-pane {
    height: 100%;
    overflow: hidden;
  }

  .tab-pane.hidden {
    display: none;
  }

</style>
