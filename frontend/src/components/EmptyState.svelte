<script>
  import { OpenCollection, PickCollectionDir } from '../../wailsjs/go/main/App'
  import { workbenchStore } from '../stores/workbench.js'

  export let errorMessage = ''

  async function handleOpen() {
    const dir = await PickCollectionDir()
    if (!dir) return
    try {
      const summaries = await OpenCollection(dir)
      workbenchStore.update((s) => ({ ...s, collectionDir: dir, summaries: summaries ?? [] }))
    } catch (e) {
      console.error('OpenCollection failed:', e)
    }
  }
</script>

<div class="empty-state">
  {#if errorMessage}
    <p class="error-notice">{errorMessage}</p>
  {/if}
  <p class="hint">No collection open.</p>
  <button class="open-btn" on:click={handleOpen}>Open collection folder</button>
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    gap: 12px;
    color: var(--text-muted);
  }

  .error-notice {
    color: var(--warn);
    font-size: 0.85rem;
    max-width: 360px;
    text-align: center;
    margin: 0;
  }

  .hint {
    margin: 0;
    font-size: 0.9rem;
  }

  .open-btn {
    padding: 8px 20px;
    background: var(--accent-primary);
    color: var(--text-on-accent-primary);
    border: none;
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.9rem;
  }

  .open-btn:hover {
    background: var(--accent-primary-hover);
  }
</style>
