<script>
  // Ask a model, refine over a few turns, then save the exchange as a markdown
  // reading source. This is a source-acquisition channel in the same role as
  // + PDF and + Web — not a chat tool that happens to save. Asking about a
  // passage you are already reading is the notebook's LLM cell, not this.
  import { onMount, onDestroy, createEventDispatcher } from 'svelte'
  import { GetLLMConfig, CreateLLMSource } from '../../wailsjs/go/main/App'
  import { streamTurn } from '../lib/llm.js'
  import { chatToMarkdown, defaultTitle } from '../lib/chat-markdown.js'

  export let chat

  const dispatch = createEventDispatcher()

  // The output is study material, not conversation. Without this the model
  // writes replies — hedges, restatements of the question, offers to go on —
  // which read badly as a document and waste extraction effort.
  const SYSTEM = [
    'You are producing study material that will be read incrementally and turned into flashcards.',
    'Answer in complete, factual prose, using short paragraphs and sub-headings where they help.',
    'Do not address the reader conversationally, do not restate the question, and do not offer follow-ups.',
  ].join(' ')

  let profiles = []
  let activeId = ''
  let input = ''
  let partial = ''
  let streaming = false
  let turn = null
  let error = ''
  let saving = false
  let lastSent = ''

  $: canCreate = chat.messages.some((m) => m.role === 'assistant') && !streaming && !saving

  onMount(async () => {
    try {
      const c = await GetLLMConfig()
      profiles = c.profiles ?? []
      activeId = c.activeProfileId ?? ''
    } catch { /* settings not configured; the host hides the entry point anyway */ }
  })
  onDestroy(() => {
    if (!turn) return
    turn.cancel()
    // Unmounting mid-stream — which is what parking the chat by opening a
    // document does — would otherwise leave the transcript ending in an
    // unanswered user turn. Native Anthropic rejects two consecutive user
    // roles, so the next send after coming back would fail. Mutating the
    // object the host still holds is what makes the drop stick past unmount.
    if (chat.messages.length && chat.messages[chat.messages.length - 1].role === 'user') {
      chat.messages = chat.messages.slice(0, -1)
    }
  })

  // The model id, not the profile name: the document's footer should say what
  // actually answered, which outlives a profile someone renames or deletes.
  function modelFor(id) {
    return profiles.find((p) => p.id === (id || activeId))?.model ?? ''
  }

  function setProfile(id) {
    chat.profileId = id || null
    chat = chat
  }

  function send() {
    const text = input.trim()
    if (!text || streaming) return
    lastSent = text
    chat.messages = [...chat.messages, { role: 'user', content: text }]
    chat = chat
    input = ''
    error = ''
    partial = ''
    streaming = true

    turn = streamTurn({
      profileId: chat.profileId,
      system: SYSTEM,
      messages: chat.messages,
      onDelta: (t) => { partial += t },
      onDone: () => finish(partial),
      onError: (m) => { error = m; abandonTurn() },
    })
  }

  function finish(text) {
    if (text) {
      chat.messages = [...chat.messages, { role: 'assistant', content: text }]
      // Recorded per completed turn, so a chat answered by two models says so.
      const m = modelFor(chat.profileId)
      if (m && !chat.models.includes(m)) chat.models = [...chat.models, m]
      chat = chat
    }
    partial = ''
    streaming = false
    turn = null
  }

  function abandonTurn() {
    // Drop the unanswered trailing user message: native Anthropic rejects two
    // consecutive user roles on the next send. Its text goes back in the input
    // so the turn can simply be retried.
    if (chat.messages.length && chat.messages[chat.messages.length - 1].role === 'user') {
      chat.messages = chat.messages.slice(0, -1)
      chat = chat
    }
    if (lastSent && !input) input = lastSent
    partial = ''
    streaming = false
    turn = null
  }

  function stop() { turn?.cancel(); abandonTurn() }

  async function create() {
    if (!canCreate) return
    const title = window.prompt('Title for this document:', defaultTitle(chat.messages))
    if (title === null) return           // cancelled
    if (!title.trim()) { error = 'A title is required.'; return }
    saving = true
    error = ''
    try {
      const md = chatToMarkdown(title, chat.messages, chat.models)
      const v = await CreateLLMSource(title, md)
      dispatch('created', v)
    } catch (e) {
      // Never lose the conversation to a failed save.
      error = String(e?.message ?? e)
    } finally {
      saving = false
    }
  }
</script>

<div class="lc-root">
  <div class="lc-bar">
    <span class="lc-title">✦ Ask a model</span>
    <select class="lc-prof" value={chat.profileId || ''} title="Model — changing it affects the next turn only"
            on:change={(e) => setProfile(e.target.value)}>
      <option value="">Default model</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <button class="lc-create" on:click={create} disabled={!canCreate}
            title="Save this conversation as a document in the reading list">
      {saving ? 'Creating…' : 'Create document'}
    </button>
  </div>

  <div class="lc-log">
    {#each chat.messages as m}
      <div class="lc-msg {m.role}"><span class="lc-role">{m.role === 'user' ? 'You' : 'Model'}</span>{m.content}</div>
    {/each}
    {#if partial}<div class="lc-msg assistant"><span class="lc-role">Model</span>{partial}</div>{/if}
    {#if !chat.messages.length && !partial}
      <div class="lc-empty">Ask for the material you want to study. When the answers are good, press Create document and it joins your reading list.</div>
    {/if}
  </div>

  {#if error}<div class="lc-error">{error}</div>{/if}

  <div class="lc-input">
    <textarea bind:value={input} rows="3" placeholder="Ask something…" disabled={streaming}
              on:keydown={(e) => { if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); send() } }}
    ></textarea>
    {#if streaming}
      <button class="lc-send" on:click={stop}>Stop</button>
    {:else}
      <button class="lc-send" on:click={send} disabled={!input.trim()} title="Ctrl+Enter">Send</button>
    {/if}
  </div>
</div>

<style>
  /* #0f1724: thematic, single-use — distinct from --surface-study (#131c29,
     the sibling root background shared by the other Study/Source full-panel
     surfaces); not byte-identical, so kept its own value under the no-tidying
     rule rather than folded in. */
  .lc-root { position: absolute; inset: 0; display: flex; flex-direction: column; background: #0f1724; }
  .lc-bar { display: flex; align-items: center; gap: 8px; padding: 8px 10px;
    border-bottom: 1px solid var(--border); background: var(--surface-bar); }
  .lc-title { color: var(--accent-text); font-weight: 600; flex: 1; font-size: 13px; }
  .lc-prof { background: var(--surface-inset); color: var(--text); border: 1px solid var(--border); border-radius: 6px;
    font-size: 11px; padding: 2px 4px; }
  .lc-create { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px;
    padding: 3px 9px; cursor: pointer; font-size: 11px; }
  .lc-create:disabled { opacity: 0.5; cursor: default; }
  .lc-log { flex: 1; min-height: 0; overflow: auto; padding: 10px; }
  .lc-empty { color: var(--text-faint); font-size: 13px; padding: 20px 4px; max-width: 46em; }
  .lc-msg { white-space: pre-wrap; font-size: 13px; line-height: 1.55; margin-bottom: 12px; max-width: 46em; }
  .lc-msg.user { color: var(--text-doc); }
  .lc-msg.assistant { color: var(--text-doc-dim); }
  .lc-role { display: block; font-size: 10px; text-transform: uppercase; letter-spacing: 0.06em;
    color: var(--text-faint); margin-bottom: 2px; }
  .lc-error { color: var(--danger); padding: 6px 10px; font-size: 12px; border-top: 1px solid var(--border); }
  .lc-input { display: flex; gap: 8px; padding: 8px 10px; border-top: 1px solid var(--border); background: var(--surface-study); }
  .lc-input textarea { flex: 1; min-width: 0; resize: vertical; background: var(--surface-sunken); color: var(--text);
    border: 1px solid var(--border); border-radius: 6px; padding: 6px 8px; font-size: 13px;
    font-family: inherit; outline: none; }
  .lc-send { align-self: flex-end; background: var(--accent); color: var(--text); border: 1px solid var(--border);
    border-radius: 6px; padding: 4px 12px; cursor: pointer; font-size: 12px; }
  .lc-send:disabled { opacity: 0.5; cursor: default; }
</style>
