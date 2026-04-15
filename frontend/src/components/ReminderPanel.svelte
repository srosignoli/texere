<script>
  import { workbenchStore } from '../stores/workbench.js'
  import { ResolveReminder, ParseFileForUI } from '../../wailsjs/go/main/App'

  export let open = false

  $: parsedFile = $workbenchStore.parsedFile
  $: reminders = parsedFile?.Reminders ?? []
  $: selectedFilePath = $workbenchStore.selectedChapterKey?.filePath ?? null

  function deriveChapterTitle(reminder) {
    const chapters = parsedFile?.Chapters ?? []
    for (let i = chapters.length - 1; i >= 0; i--) {
      if (chapters[i].LineStart <= reminder.Line) return chapters[i].Title
    }
    return ''
  }

  function deriveEffectiveCtx(reminder) {
    const chapters = parsedFile?.Chapters ?? []
    for (const ch of chapters) {
      for (let i = ch.Nodes.length - 1; i >= 0; i--) {
        if (ch.Nodes[i].Line <= reminder.Line) return ch.Nodes[i].EffectiveCtx ?? []
      }
    }
    return []
  }

  async function handleResolve(reminder) {
    try {
      await ResolveReminder(reminder.File, reminder.Line)
      // Refresh parsedFile so badge auto-decrements
      const freshResult = await ParseFileForUI(reminder.File)
      if (freshResult && !Array.isArray(freshResult)) {
        workbenchStore.update((s) => ({ ...s, parsedFile: freshResult }))
      }
    } catch (e) {
      console.error('ResolveReminder failed:', e)
    }
  }
</script>

{#if open}
  <div class="panel-overlay">
    <div class="panel">
      <div class="panel-header">
        <span class="panel-title">Reminders</span>
        <button class="close-btn" on:click={() => (open = false)}>✕</button>
      </div>

      <div class="panel-body">
        {#if reminders.length === 0}
          <div class="empty-hint">No open reminders</div>
        {:else}
          {#each reminders as reminder (reminder.Line)}
            {@const chapterTitle = deriveChapterTitle(reminder)}
            {@const ctx = deriveEffectiveCtx(reminder)}
            {@const isDifferentFile = reminder.File !== selectedFilePath}
            <div class="reminder-item">
              <div class="reminder-meta">
                {#if chapterTitle}<span class="meta-chapter">{chapterTitle}</span>{/if}
                {#if ctx.length > 0}
                  <span class="meta-ctx">{ctx.join(', ')}</span>
                {/if}
              </div>
              <p class="reminder-text">{reminder.Text}</p>
              <div class="reminder-actions">
                {#if isDifferentFile}
                  <span class="different-file-hint" title={reminder.File}>⚠ Different file</span>
                {/if}
                <button class="resolve-btn" on:click={() => handleResolve(reminder)}>
                  Resolve
                </button>
              </div>
            </div>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .panel-overlay {
    position: fixed;
    inset: 0;
    z-index: 200;
    pointer-events: none;
  }

  .panel {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 320px;
    background: #1a2535;
    border-left: 1px solid #2a3a4a;
    display: flex;
    flex-direction: column;
    pointer-events: all;
    box-shadow: -4px 0 16px rgba(0, 0, 0, 0.4);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 10px 14px;
    border-bottom: 1px solid #2a3a4a;
    flex-shrink: 0;
  }

  .panel-title {
    font-size: 0.88rem;
    font-weight: 600;
    color: #e8eaf0;
  }

  .close-btn {
    background: none;
    border: none;
    color: #5a7a9a;
    cursor: pointer;
    font-size: 0.9rem;
    padding: 2px 6px;
  }

  .close-btn:hover {
    color: #e8eaf0;
  }

  .panel-body {
    flex: 1;
    overflow-y: auto;
    padding: 8px;
  }

  .empty-hint {
    padding: 20px;
    text-align: center;
    color: #4a5a6a;
    font-size: 0.82rem;
  }

  .reminder-item {
    background: #1e2f40;
    border: 1px solid #2a3a4a;
    border-radius: 6px;
    padding: 10px 12px;
    margin-bottom: 8px;
  }

  .reminder-meta {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    margin-bottom: 4px;
  }

  .meta-chapter {
    font-size: 0.72rem;
    font-weight: 600;
    color: #5a7a9a;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .meta-ctx {
    font-size: 0.72rem;
    color: #4a5a6a;
    font-style: italic;
  }

  .reminder-text {
    margin: 0 0 8px;
    font-size: 0.84rem;
    color: #f59e0b;
    font-weight: 600;
  }

  .reminder-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
  }

  .different-file-hint {
    font-size: 0.75rem;
    color: #f59e0b;
  }

  .resolve-btn {
    padding: 4px 12px;
    background: #1e3a5a;
    border: 1px solid #2563eb;
    border-radius: 4px;
    color: #93c5fd;
    font-size: 0.8rem;
    cursor: pointer;
  }

  .resolve-btn:hover {
    background: #1e4a7a;
  }
</style>
