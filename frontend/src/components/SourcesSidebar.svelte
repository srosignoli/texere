<script>
  // The reading library: a collapsible sidebar that stays visible while you
  // read, so switching documents or checking what you have already extracted
  // does not mean closing what you are in.
  import { onMount, createEventDispatcher } from 'svelte'
  import {
    ImportTextSource, ImportPdfSource, ImportWebSource, GetReadingList, GetReadingExtracts,
  } from '../../wailsjs/go/main/App'
  import { filterSources } from '../lib/sources-filter.js'

  const dispatch = createEventDispatcher()

  let items = []
  let extractsByReading = {}
  let query = ''
  let expanded = {}     // reading id -> manually opened
  let collapsed = false
  let error = ''

  $: rows = filterSources(items, extractsByReading, query)

  async function load() {
    error = ''
    try {
      items = (await GetReadingList()) ?? []
      extractsByReading = (await GetReadingExtracts()) ?? {}
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
  function importWeb() {
    const url = window.prompt('Article URL:')
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
            </div>

            {#if open}
              <ul class="sb-extracts">
                {#each extractsFor(row) as ex (ex.id)}
                  <li>
                    <button class="sb-extract"
                            on:click={() => dispatch('peek', { reading: row.reading, anchor: ex.anchor })}
                            title={ex.text}>⤷ {ex.text}</button>
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
    background: #131c29; border-right: 1px solid #2a3a4a; min-height: 0; }
  .sb-rail { width: 22px; flex: 0 0 auto; display: flex; justify-content: center; padding-top: 8px;
    background: #131c29; border-right: 1px solid #2a3a4a; }
  .sb-bar { display: flex; align-items: center; gap: 6px; padding: 8px 10px;
    border-bottom: 1px solid #2a3a4a; background: #161f2e; }
  .sb-title { color: #7dd3fc; font-weight: 600; flex: 1; font-size: 13px; }
  .sb-btn { background: #1e3a5a; color: #e8eaf0; border: 1px solid #2a3a4a; border-radius: 6px;
    padding: 3px 7px; cursor: pointer; font-size: 11px; }
  .sb-toggle { background: none; border: none; color: #8899aa; font-size: 15px; cursor: pointer; padding: 0 4px; }
  .sb-search { margin: 8px 10px; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px;
    color: #e8eaf0; font-size: 12px; padding: 5px 8px; outline: none; }
  .sb-search::placeholder { color: #5d7186; }
  .sb-error { color: #ff8080; padding: 6px 10px; font-size: 12px; }
  .sb-empty { color: #8aa; padding: 14px 10px; font-size: 13px; }
  .sb-list { list-style: none; margin: 0; padding: 0; overflow: auto; flex: 1; min-height: 0; }
  .sb-doc { border-bottom: 1px solid #1d2a3a; }
  .sb-docrow { display: flex; align-items: center; gap: 6px; padding: 6px 10px; }
  .sb-tri { background: none; border: none; color: #8899aa; cursor: pointer; font-size: 11px; padding: 0 2px; }
  .sb-name { flex: 1; min-width: 0; background: none; border: none; color: #cdd6e0; font-size: 13px;
    text-align: left; cursor: pointer; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-name:hover { color: #7dd3fc; }
  .sb-pct { color: #7ecfa0; font-size: 12px; }
  .sb-tag { font-size: 10px; border-radius: 3px; padding: 1px 5px; }
  .sb-tag.due { background: #4a3a1a; color: #e0b000; }
  .sb-tag.done { background: #1a3a2a; color: #7ecfa0; }
  .sb-extracts { list-style: none; margin: 0 0 4px; padding: 0 0 0 22px; }
  .sb-extract { background: none; border: none; color: #9fb3c8; font-size: 12px; text-align: left;
    cursor: pointer; padding: 3px 8px 3px 0; display: block; width: 100%;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sb-extract:hover { color: #7dd3fc; }
  .sb-noextracts { color: #5d7186; font-size: 11px; padding: 2px 0 4px; }
</style>
