<script>
  // A plain-language question turned into a graph query.
  //
  // Deliberately a SEPARATE input from whatever query bar it sits above. Typing
  // prose into the query bar itself fought the bar: the omnibar searches as you
  // type and opens the directive autocomplete, so asking a question produced a
  // dropdown of matches for those literal words before the button could be
  // reached. Nothing typed here is ever searched.
  //
  // The host decides what to do with the result — the omnibar drops it in its
  // bar (which then searches as you type), the explorer fills its bar and waits
  // for an explicit submit. This component only produces the query.
  //
  // Renders nothing at all when no LLM profile is configured: a user who has
  // never set a provider up should not see a control they cannot use.

  import { onMount, createEventDispatcher } from 'svelte'
  import {
    TranslateQuery, ListQueryExamples, GetQueryLLMProfile, SetQueryLLMProfile, GetLLMConfig,
  } from '../../wailsjs/go/main/App'
  import { buildQueryPrompt, stripToQuery, validateQuery } from '../lib/query-prompt.js'
  import { queryHints, refreshQueryHints } from '../stores/queryHints.js'
  import { appendLog } from '../stores/logPanel.js'

  export let placeholder = 'Ask in plain language, e.g. what is in my kubernetes notes?'

  const dispatch = createEventDispatcher()

  let llmProfiles = []
  let queryProfileId = ''
  let examples = []
  let translating = false
  let question = ''
  let refusal = null       // { query, unknown: string[] } while a bad query is held back
  let translateError = ''  // provider failure, shown here rather than only logged

  $: canTranslate = llmProfiles.length > 0

  onMount(async () => {
    try { const c = await GetLLMConfig(); llmProfiles = c.profiles ?? [] } catch { /* not configured */ }
    try { queryProfileId = (await GetQueryLLMProfile()) ?? '' } catch { /* default */ }
    try { examples = (await ListQueryExamples()) ?? [] } catch { /* prompt degrades to grammar only */ }
    // A profile deleted since it was chosen must fall back, not persist a stale
    // name — providerFor resolves an unknown id to an error, not to the default.
    if (queryProfileId && !llmProfiles.some((p) => p.id === queryProfileId)) queryProfileId = ''
  })

  function setQueryProfile(id) {
    queryProfileId = id || ''
    SetQueryLLMProfile(queryProfileId).catch(() => {})
  }

  export function reset() {
    question = ''
    refusal = null
    translateError = ''
  }

  async function translate() {
    const q0 = question.trim()
    if (!q0 || translating) return
    translating = true
    refusal = null
    translateError = ''
    // The prompt is worthless without the user's real chapter and context names,
    // and the lazy on:focus load elsewhere can be skipped for a whole session.
    // Fetch here rather than trust that something else did.
    const h = $queryHints
    if (!h.chapter.length && !h.context.length && !h.arrow.length) {
      await refreshQueryHints()
    }
    try {
      const raw = await TranslateQuery(queryProfileId, q0, buildQueryPrompt(examples, $queryHints))
      const q = stripToQuery(raw)
      const v = validateQuery(q)
      if (!v.ok) { refusal = { query: q, unknown: v.unknown }; return }
      accept(q)
    } catch (e) {
      // Surface it where the user is looking. Logging alone made a failure
      // indistinguishable from nothing happening: the question stayed put, the
      // button re-enabled, and the only trace was a line in a usually-closed panel.
      translateError = String(e?.message ?? e)
      appendLog([{ severity: 'error', source: 'search', message: `translate: ${translateError}` }])
    } finally {
      translating = false
    }
  }

  function accept(q) {
    refusal = null
    translateError = ''
    dispatch('translated', q)
  }
</script>

{#if canTranslate}
  <div class="ask-row">
    <span class="ask-icon" aria-hidden="true">✨</span>
    <input
      class="ask-input"
      type="text"
      bind:value={question}
      {placeholder}
      autocomplete="off"
      spellcheck="false"
      aria-label="Ask a question in plain language"
      on:keydown={(e) => {
        // Stop Enter reaching the host, which would run its query bar or accept
        // an autocomplete item.
        if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); translate() }
      }} />
    <select class="ask-prof" value={queryProfileId}
            on:change={(e) => setQueryProfile(e.target.value)} title="Model used to translate">
      <option value="">Default model</option>
      {#each llmProfiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <button class="ask-go" type="button" on:click={translate} disabled={translating}
            title="Turn this question into a query">
      {translating ? '…' : '✨'}
    </button>
  </div>

  {#if translateError}
    <div class="ask-note">
      <div><b>Could not translate:</b> {translateError}</div>
      <div class="ask-actions">
        <button type="button" on:click={translate}>Retry</button>
        <button type="button" on:click={() => (translateError = '')}>Dismiss</button>
      </div>
    </div>
  {/if}

  {#if refusal}
    <div class="ask-note">
      <div><b>Not usable here:</b> {refusal.unknown.join(', ')} — upstream searchN4L syntax texere does not execute.</div>
      <code>{refusal.query}</code>
      <div class="ask-actions">
        <button type="button" on:click={translate}>Retry</button>
        <button type="button" on:click={() => accept(refusal.query)}>Use anyway</button>
        <button type="button" on:click={() => (refusal = null)}>Dismiss</button>
      </div>
    </div>
  {/if}
{/if}

<style>
  .ask-row { display: flex; align-items: center; gap: 8px; padding: 6px 10px; }
  .ask-icon { color: var(--accent-text); font-size: 13px; }
  .ask-input { flex: 1 1 auto; min-width: 0; background: transparent; border: none; outline: none; color: var(--text); font-size: 13px; }
  .ask-input::placeholder { color: var(--text-faint); }
  .ask-prof { background: var(--surface-inset); color: var(--text); border: 1px solid var(--border); border-radius: 6px; font-size: 11px; padding: 2px 4px; }
  .ask-go { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 2px 8px; cursor: pointer; }
  .ask-go:disabled { opacity: 0.6; cursor: default; }
  .ask-note { padding: 6px 10px; border-top: 1px solid var(--border); background: var(--surface-inset); color: var(--text); font-size: 12px; }
  .ask-note code { display: block; margin: 4px 0; color: #fbbf24; }
  .ask-actions { display: flex; gap: 6px; }
  .ask-actions button { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 11px; }
</style>
