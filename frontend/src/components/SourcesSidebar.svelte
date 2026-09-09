<script>
  import { askText } from '../lib/dialog.js'
  // The reading library: a collapsible sidebar that stays visible while you
  // read, so switching documents or checking what you have already extracted
  // does not mean closing what you are in.
  import { onMount, createEventDispatcher } from 'svelte'
  import {
    ImportTextSource, ImportPdfSource, ImportWebSource, GetReadingList, GetReadingExtracts, GetLLMConfig,
  } from '../../wailsjs/go/main/App'
  import { filterSources } from '../lib/sources-filter.js'

  const dispatch = createEventDispatcher()

  let items = []
  let extractsByReading = {}
  let query = ''
  let expanded = {}     // reading id -> manually opened
  let collapsed = false
  let error = ''
  let hasLLM = false

  // Shown only while a chat is parked in Study mode, so it can be reopened.
  export let chatLive = false

  $: rows = filterSources(items, extractsByReading, query)

  async function load() {
    error = ''
    try {
      items = (await GetReadingList()) ?? []
      extractsByReading = (await GetReadingExtracts()) ?? {}
      // Rendered only when a profile exists: a user who has never configured a
      // provider should not see a control they cannot use (as AskBox does).
      try { hasLLM = ((await GetLLMConfig())?.profiles ?? []).length > 0 } catch { hasLLM = false }
    } catch (e) {
      error = String(e?.message ?? e)
    }
  }

  // A never-scheduled source has due===0 (NULL); StudyCounts and StudySurface
  // both count that as due, so the badge must too.
  const dueNow = (v) => !v.done && (v.due === 0 || v.due * 1000 <= Date.now())

  // `expanded` is read from the markup below, not only from here. Svelte
  // instruments an assignment only when the template references the variable,
  // so while this state lived purely inside helper functions, clicking a
  // triangle mutated it and scheduled no re-render at all — the tree never
  // opened, and a later unrelated update made every clicked row snap open.
  //
  // undefined means "never touched by hand", so a row a search auto-expanded
  // still falls back to autoExpand, and a click can now close it.
  function toggle(row) {
    const id = row.reading.id
    expanded[id] = !(expanded[id] ?? row.autoExpand)
    expanded = expanded
  }
  function extractsFor(row) {
    return row.autoExpand ? row.extracts : (extractsByReading[row.reading.id] ?? [])
  }

  async function importVia(fn, arg) {
    error = ''
    try {
      const v = arg === undefined ? await fn() : await fn(arg)
      if (v) { await load(); dispatch('open', v) }
    } catch (e) { error = String(e?.message ?? e) }
  }
  const importText = () => importVia(ImportTextSource)
  const importPdf = () => importVia(ImportPdfSource)
  async function importWeb() {
    const url = await askText('Article URL', '', { placeholder: 'https://…', confirmLabel: 'Import' })
    if (url) importVia(ImportWebSource, url.trim())
  }

  export function refresh() { return load() }

  onMount(load)
</script>

{#if collapsed}
  <div class="sb-rail">
    <button class="sb-toggle" on:click={() => (collapsed = false)} title="Show sources">›</button>
  </div>
{:else}
  <div class="sb-root">
    <div class="sb-bar">
      <span class="sb-title">Sources</span>
      <button class="sb-btn" on:click={importText} title="Import a text or markdown file">+ Text</button>
      <button class="sb-btn" on:click={importPdf} title="Import a PDF">+ PDF</button>
      <button class="sb-btn" on:click={importWeb} title="Import a web article">+ Web</button>
      {#if hasLLM}
        <button class="sb-btn" on:click={() => dispatch('newChat')} title="Ask a model and save the answers as a document">+ LLM</button>
      {/if}
      <button class="sb-toggle" on:click={() => (collapsed = true)} title="Hide sources">‹</button>
    </div>

    <input class="sb-search" type="text" bind:value={query} placeholder="Search titles and extracts…"
           autocomplete="off" spellcheck="false" aria-label="Search sources" />

    {#if error}<div class="sb-error">{error}</div>{/if}

    {#if rows.length === 0}
      <div class="sb-empty">
        {query ? 'Nothing matches.' : 'No sources yet. Import a .txt, .md, or .html file to start reading.'}
      </div>
    {:else}
      {#if chatLive}
        <button class="sb-chatrow" on:click={() => dispatch('openChat')}>✦ Chat in progress</button>
      {/if}
      <ul class="sb-list">
        {#each rows as row (row.reading.id)}
          {@const open = expanded[row.reading.id] ?? row.autoExpand}
          <li class="sb-doc">
            <div class="sb-docrow">
              <!-- Its own control: the triangle reveals the tree, the title opens
                   the document. Making the whole row toggle would mean you could
                   not open a document without also expanding it. -->
              <button class="sb-tri" on:click|stopPropagation={() => toggle(row)}
                      aria-label={open ? 'Hide extracts' : 'Show extracts'}>
                {open ? '▾' : '▸'}
              </button>
              <button class="sb-name" on:click={() => dispatch('open', row.reading)} title={row.reading.title}>
                {row.reading.title}
              </button>
              <span class="sb-pct">{row.reading.percent}%</span>
              {#if row.reading.done}<span class="sb-tag done">done</span>
              {:else if dueNow(row.reading)}<span class="sb-tag due">due</span>{/if}
              {#if row.reading.medium === 'llm'}<span class="sb-tag llm">llm</span>{/if}
            </div>

            {#if open}
              <ul class="sb-extracts">
                {#each extractsFor(row) as ex (ex.id)}
                  <li>
                    <button class="sb-extract"
                            on:click={() => dispatch('peek', { reading: row.reading, anchor: ex.anchor })}
                            title={ex.promoted ? ex.text + ' — already promoted to a card' : ex.text}>
                      <span class="sb-extract-text">⤷ {ex.text}</span>
                      {#if ex.promoted}<span class="sb-card">card</span>{/if}
                    </button>
                  </li>
                {:else}
                  <li class="sb-noextracts">No extracts yet.</li>
                {/each}
              </ul>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .sb-root { width: 300px; flex: 0 0 auto; display: flex; flex-direction: column;
    background: var(--surface-study); border-right: 1px solid var(--border); min-height: 0; }
  .sb-rail { width: 22px; flex: 0 0 auto; display: flex; justify-content: center; padding-top: 8px;
    background: var(--surface-study); border-right: 1px solid var(--border); }
  .sb-bar { display: flex; align-items: center; gap: 6px; padding: 8px 10px;
    border-bottom: 1px solid var(--border); background: var(--surface-bar); }
  .sb-title { color: var(--accent-text); font-weight: 600; flex: 1; font-size: 13px; }
  .sb-btn { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px;
    padding: 3px 7px; cursor: pointer; font-size: 11px; }
  .sb-toggle { background: none; border: none; color: var(--text-muted); font-size: 15px; cursor: pointer; padding: 0 4px; }
  .sb-search { margin: 8px 10px; background: var(--surface-sunken); border: 1px solid var(--border); border-radius: 6px;
    color: var(--text); font-size: 12px; padding: 5px 8px; outline: none; }
  .sb-search::placeholder { color: var(--text-faint); }
  .sb-error { color: var(--danger); padding: 6px 10px; font-size: 12px; }
  .sb-empty { color: var(--text-quiet); padding: 14px 10px; font-size: 13px; }
  .sb-list { list-style: none; margin: 0; padding: 0; overflow: auto; flex: 1; min-height: 0; }
  .sb-doc { border-bottom: 1px solid var(--border-faint); }
  .sb-chatrow { display: block; width: 100%; text-align: left; background: var(--surface-highlight);
    border: none; border-bottom: 1px solid var(--border); color: var(--accent-text); font-size: 12px;
    padding: 6px 10px; cursor: pointer; }
  .sb-docrow { display: flex; align-items: center; gap: 6px; padding: 6px 10px; }
  .sb-tri { background: none; border: none; color: var(--text-muted); cursor: pointer; font-size: 11px; padding: 0 2px; }
  .sb-name { flex: 1; min-width: 0; background: none; border: none; color: var(--text-doc); font-size: 13px;
    text-align: left; cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-name:hover { color: var(--accent-text); }
  .sb-pct { color: var(--ok); font-size: 12px; }
  .sb-tag { font-size: 10px; border-radius: 3px; padding: 1px 5px; }
  /* .sb-tag.due/.done/.llm: three semantic, coupled bg+fg status badges — due
     (amber), done (green), llm (purple) — each kept as one literal unit even
     where one half byte-matches a token (.done's #7ecfa0 == --ok), because
     tokenizing only the text while the background stays a fixed literal would
     break the pairing's contrast the moment a theme diverges from Blue. #e0b000
     also drives StatsPanel's chart series (its stat-value colour moved to
     var(--stat-high), style.css:154); #b9a3e3 the same for its purple series
     — see StatsPanel for the semantic reasoning. */
  .sb-tag.due { background: #4a3a1a; color: #e0b000; }
  .sb-tag.done { background: #1a3a2a; color: #7ecfa0; }
  .sb-tag.llm { background: #2c2340; color: #b9a3e3; }
  .sb-extracts { list-style: none; margin: 0 0 4px; padding: 0 0 0 22px; }
  .sb-extract { background: none; border: none; color: var(--text-doc-dim); font-size: 12px; text-align: left;
    cursor: pointer; padding: 3px 8px 3px 0; display: flex; align-items: center; gap: 6px; width: 100%; }
  /* min-width:0 is load-bearing: a flex item will not shrink below its content
     width without it, so the text would never ellipsise and would instead push
     the badge past the sidebar's edge — which is exactly how the badge went
     missing when it was an inline span inside a truncating block. */
  .sb-extract-text { flex: 1 1 auto; min-width: 0;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-extract:hover { color: var(--accent-text); }
  /* A promoted passage stays in the tree — it is still something you took from
     this document — but reads differently from one still awaiting processing.
     flex:0 0 auto keeps it whole while the text beside it truncates. */
  .sb-card { flex: 0 0 auto; font-size: 9px; border-radius: 3px; padding: 0 4px;
    background: var(--border); color: var(--text-badge); }
  .sb-noextracts { color: var(--text-faint); font-size: 11px; padding: 2px 0 4px; }
</style>
