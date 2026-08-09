<script>
  import { onMount } from 'svelte'
  import { GetStudyCounts, GetExtractQueue, GetReadingList, GetDueQueue, GradeItem } from '../../wailsjs/go/main/App'
  import { studyFocus } from '../stores/study.js'
  import { activeReading } from '../stores/reading.js'
  import ReadingView from './ReadingView.svelte'
  import SourcesSidebar from './SourcesSidebar.svelte'
  import LLMChatPane from './LLMChatPane.svelte'
  import ProcessView from './ProcessView.svelte'
  import RecallCard from './RecallCard.svelte'

  export let onClose = null

  let counts = { read: 0, process: 0, recall: 0 }
  let error = ''

  // Process state
  let processId = null
  // Recall state
  let recallCard = null
  let recallShownAt = 0
  // Read state: activeReading store + library fallback
  let peekAt = null   // anchor for an extract peek, cleared on a normal open
  let sidebar = null  // SourcesSidebar instance, so closing a document can reload its tree
  let readingView = null
  // The chat lives here, not inside the reading area's {#key} block, so it
  // survives reading a document and coming back. It dies with Study mode.
  let chat = null

  // The {#key} expression below, so we can tell whether a click will actually
  // remount the view. Flushing when nothing remounts would mark a still-live
  // ReadingView as closed, and its real close would then persist nothing.
  const readingKey = (r, anchor) => (r?.id ?? '') + '|' + (anchor ?? '')

  // Open (anchor null) or peek a document from the sidebar. The sidebar's rows
  // come from a cache filled on mount, so re-read the reading first: switching
  // away from a document and back would otherwise reopen it at a stale position
  // and then write that stale value back over the good one on close.
  // Two clicks inside one flush's IPC round trip would otherwise let the slower
  // call finish last and display the document the user did NOT click last, so
  // each call takes a ticket and drops out once a newer one has started.
  let switchSeq = 0
  let pendingFlush = null

  async function switchTo(r, anchor) {
    const seq = ++switchSeq
    if (readingKey(r, anchor) !== readingKey($activeReading, peekAt)) {
      pendingFlush = readingView?.flush?.() ?? null
    }
    // Await whatever flush is outstanding, not just one this call started: a
    // click that skips the flush can still race an earlier one still in flight,
    // and reading back a position before its write lands is the whole bug.
    if (pendingFlush) await pendingFlush
    if (seq !== switchSeq) return
    const fresh = await freshReading(r)
    if (seq !== switchSeq) return
    peekAt = anchor
    activeReading.set(fresh)
    await sidebar?.refresh()
  }

  async function newChat() {
    if (chat && chat.messages.length && !window.confirm('Discard the chat in progress?')) return
    await readingView?.flush?.()
    peekAt = null
    activeReading.set(null)
    chat = { messages: [], profileId: null, models: [] }
  }

  // $activeReading wins over a parked chat in the markup, so clearing it is
  // what brings the chat back into view.
  async function openChat() {
    await readingView?.flush?.()
    peekAt = null
    activeReading.set(null)
  }

  // The saved document behaves like any other source from here on.
  async function chatCreated(v) {
    chat = null
    await sidebar?.refresh()
    if (v) await switchTo(v, null)
  }

  async function freshReading(r) {
    try {
      const list = await GetReadingList()
      return (list ?? []).find((x) => x.id === r.id) ?? r
    } catch { return r }
  }

  async function refresh() {
    try { counts = (await GetStudyCounts()) ?? counts } catch (e) { error = String(e?.message ?? e) }
  }

  async function loadProcess() {
    try {
      const q = await GetExtractQueue(1)
      processId = q && q.length ? q[0].id : null
    } catch (e) { error = String(e?.message ?? e) }
  }
  async function loadRecall() {
    try {
      const q = await GetDueQueue(1)
      recallCard = q && q.length ? q[0] : null
      recallShownAt = Date.now()
    } catch (e) { error = String(e?.message ?? e) }
  }
  async function loadRead() {
    try {
      if ($activeReading) return
      const list = await GetReadingList()
      const due = (list ?? []).find((r) => !r.done && (r.due === 0 || r.due * 1000 <= Date.now()))
      if (due) activeReading.set(due) // open the next due source; else SourcesSidebar shows the library
    } catch (e) { error = String(e?.message ?? e) }
  }

  // Drive the active focus.
  async function enter(focus) {
    error = ''
    studyFocus.set(focus)
    await refresh()
    if (focus === 'process' || focus === 'next') await loadProcess()
    if (focus === 'recall' || focus === 'next') await loadRecall()
    if (focus === 'read' || focus === 'next') await loadRead()
  }

  // "Next": rotate read → process → recall, picking the first non-empty.
  function nextKind() {
    if (counts.read > 0 || $activeReading) return 'read'
    if (counts.process > 0) return 'process'
    if (counts.recall > 0) return 'recall'
    return 'empty'
  }

  async function onProcessAdvance() { await refresh(); await loadProcess(); if ($studyFocus === 'next' && !processId) await advanceNext() }
  async function onRecallGrade(n) {
    if (!recallCard) return
    try {
      recallCard = (await GradeItem(recallCard.id, n, Date.now() - recallShownAt)) ?? null
      recallShownAt = Date.now()
    } catch (e) { error = String(e?.message ?? e) }
    await refresh()
    if ($studyFocus === 'next' && !recallCard) await advanceNext()
  }
  async function advanceNext() { await enter('next') }

  // A chat is persisted nowhere, so closing Study destroys it. Both exits ask
  // first: this is the only place the conversation can be lost without the user
  // having chosen to lose it.
  function closeStudy() {
    if (chat && chat.messages.length && !window.confirm('Discard the chat in progress?')) return
    onClose?.()
  }

  function onKey(e) { if (e.key === 'Escape') closeStudy() }
  onMount(() => enter($studyFocus))
</script>

<svelte:window on:keydown={onKey} />

<div class="ss-root" role="dialog" aria-label="Study">
  <div class="ss-bar">
    <span class="ss-title">Study</span>
    {#each [['next','Next'],['read','Read'],['process','Process'],['recall','Recall']] as [k, label]}
      <button class="ss-tab" class:active={$studyFocus === k} on:click={() => enter(k)}>
        {label}{#if k === 'read' && counts.read}<sup>{counts.read}</sup>{:else if k === 'process' && counts.process}<sup>{counts.process}</sup>{:else if k === 'recall' && counts.recall}<sup>{counts.recall}</sup>{/if}
      </button>
    {/each}
    <span class="ss-spacer"></span>
    <button class="ss-close" on:click={closeStudy} title="Close (Esc)">×</button>
  </div>
  {#if error}<div class="ss-error">{error}</div>{/if}

  <div class="ss-body">
    {#if $studyFocus === 'read' || ($studyFocus === 'next' && nextKind() === 'read')}
      <div class="ss-read">
        <SourcesSidebar
          bind:this={sidebar}
          chatLive={chat !== null}
          on:newChat={newChat}
          on:openChat={openChat}
          on:open={(e) => switchTo(e.detail, null)}
          on:peek={(e) => switchTo(e.detail.reading, e.detail.anchor)} />
        <div class="ss-reading">
          {#if $activeReading}
            {#key ($activeReading.id ?? '') + '|' + (peekAt ?? '')}
              <ReadingView bind:this={readingView} reading={$activeReading} {peekAt}
                onClose={() => { peekAt = null; activeReading.set(null); sidebar?.refresh(); enter('next') }} />
            {/key}
          {:else if chat}
            <LLMChatPane bind:chat on:created={(e) => chatCreated(e.detail)} />
          {:else}
            <div class="ss-empty">Pick a document to read.</div>
          {/if}
        </div>
      </div>
    {:else if $studyFocus === 'process' || ($studyFocus === 'next' && nextKind() === 'process')}
      {#if processId}
        {#key processId}<ProcessView extractId={processId} onAdvance={onProcessAdvance} />{/key}
      {:else}
        <div class="ss-empty">No extracts to process.</div>
      {/if}
    {:else if $studyFocus === 'recall' || ($studyFocus === 'next' && nextKind() === 'recall')}
      <RecallCard card={recallCard} onGrade={onRecallGrade} />
    {:else}
      <div class="ss-empty">Nothing due. 🎉</div>
    {/if}
  </div>
</div>

<style>
  .ss-root { position: absolute; inset: 0; z-index: 60; display: flex; flex-direction: column; background: #131c29; }
  .ss-bar { display: flex; align-items: center; gap: 6px; padding: 8px 12px; border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .ss-title { color: #7dd3fc; font-weight: 600; margin-right: 8px; }
  .ss-tab { background: none; border: 1px solid transparent; color: #8aa; border-radius: 6px; padding: 3px 10px; cursor: pointer; font-size: 12px; }
  .ss-tab.active { background: #1e3a5a; color: #e8eaf0; border-color: #2a3a4a; }
  .ss-tab sup { color: #7ecfa0; margin-left: 2px; }
  .ss-spacer { flex: 1; }
  .ss-close { background: none; border: none; color: #8899aa; font-size: 18px; cursor: pointer; }
  .ss-error { color: #ff8080; padding: 6px 12px; font-size: 12px; }
  .ss-body { flex: 1; min-height: 0; position: relative; }
  .ss-read { position: absolute; inset: 0; display: flex; min-height: 0; }
  .ss-reading { position: relative; flex: 1; min-width: 0; }
  .ss-empty { display: flex; align-items: center; justify-content: center; height: 100%; color: #8aa; }
</style>
