<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetLLMConfig } from '../../../wailsjs/go/main/App'
  import { buildSystem, streamTurn } from '../../lib/llm.js'

  export let cell
  export let context = null
  export let cells = []
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false

  let profiles = []
  let input = ''
  let streaming = false
  let partial = ''
  let turn = null
  let error = ''
  let lastSent = ''

  onMount(async () => { try { const c = await GetLLMConfig(); profiles = c.profiles ?? [] } catch { /* settings not configured */ } })
  onDestroy(() => turn?.cancel())

  $: attached = (cell.attachments ?? []).map((id) => cells.find((c) => c.id === id)).filter(Boolean)
  $: hasProvider = profiles.length > 0

  function persist() { onChange(cell) }
  function setMode(m) { cell.mode = m; cell = cell; persist() }
  function setProfile(id) { cell.profileId = id || null; cell = cell; persist() }
  function toggleAttach(id) {
    const a = new Set(cell.attachments ?? [])
    if (a.has(id)) a.delete(id); else a.add(id)
    cell.attachments = [...a]; cell = cell; persist()
  }

  function send() {
    const text = input.trim()
    if (!text || streaming) return
    lastSent = text
    cell.messages = [...cell.messages, { role: 'user', content: text }]
    cell = cell
    input = ''
    error = ''
    partial = ''
    streaming = true
    const system = buildSystem(cell.mode, context, attached)
    turn = streamTurn({
      profileId: cell.profileId,
      system,
      messages: cell.messages,
      onDelta: (t) => { partial += t },
      onDone: () => { finish(partial) },
      onError: (m) => { error = m; abandonTurn() },
    })
  }

  function finish(text) {
    if (text) { cell.messages = [...cell.messages, { role: 'assistant', content: text }]; cell = cell; persist() }
    partial = ''
    streaming = false
    turn = null
  }

  function abandonTurn() {
    // Remove the unanswered trailing user message so the transcript never ends
    // with a dangling user turn (native Anthropic rejects consecutive user
    // roles on the next send), and restore its text to the input for retry.
    if (cell.messages.length && cell.messages[cell.messages.length - 1].role === 'user') {
      cell.messages = cell.messages.slice(0, -1)
      cell = cell
    }
    if (lastSent && !input) input = lastSent
    partial = ''
    streaming = false
    turn = null
  }

  function stop() { turn?.cancel(); abandonTurn() }
  function onKey(e) { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) { e.preventDefault(); send() } }
</script>

<div class="llm-cell">
  <div class="llm-bar">
    <div class="modes">
      <button class:active={cell.mode === 'ask'} on:click={() => setMode('ask')} disabled={readOnly}>Ask</button>
      <button class:active={cell.mode === 'tutor'} on:click={() => setMode('tutor')} disabled={readOnly}>Tutor</button>
    </div>
    <select class="prof" value={cell.profileId || ''} on:change={(e) => setProfile(e.target.value)} disabled={readOnly} title="Model profile">
      <option value="">active default</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <span class="spacer"></span>
    {#if !readOnly}<button class="del" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if !readOnly && cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm')).length}
    <div class="attach">
      <span>Attach context:</span>
      {#each cells.filter((c) => c.id !== cell.id && (c.type === 'n4l' || c.type === 'llm')) as c}
        <label><input type="checkbox" checked={(cell.attachments ?? []).includes(c.id)} on:change={() => toggleAttach(c.id)} /> {c.type === 'n4l' ? 'draft' : 'llm'} {c.id.slice(-4)}</label>
      {/each}
    </div>
  {/if}

  <div class="transcript">
    {#each cell.messages as m}
      <div class="msg {m.role}"><span class="who">{m.role}</span><div class="body">{m.content}</div></div>
    {/each}
    {#if streaming}<div class="msg assistant"><span class="who">assistant</span><div class="body">{partial}<span class="caret">▋</span></div></div>{/if}
    {#if error}<div class="llm-error">{error}</div>{/if}
  </div>

  {#if !readOnly}
    {#if !hasProvider}
      <div class="llm-hint">No LLM provider configured — open ⚙ Models to add one.</div>
    {:else}
      <div class="composer">
        <textarea bind:value={input} on:keydown={onKey} placeholder={cell.mode === 'tutor' ? 'Explain your understanding, or draft a card…' : 'Ask…'} rows="2" disabled={streaming}></textarea>
        {#if streaming}<button class="stop" on:click={stop}>Stop</button>{:else}<button class="send" on:click={send}>Send</button>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .llm-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .llm-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid #1c2a3a; }
  .modes button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 10px; cursor: pointer; font-size: 12px; }
  .modes button.active { background: #244a72; color: #e8eaf0; }
  /* appearance:none lets WebKitGTK apply the app's dark-blue background to the
     select and its popup instead of the UA grey; data-URI chevron replaces the
     removed native arrow. */
  .prof {
    -webkit-appearance: none; appearance: none;
    background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #cdd6e2; font-size: 12px; padding: 2px 22px 2px 6px;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath fill='%23cdd6e2' d='M0 0l5 6 5-6z'/%3E%3C/svg%3E");
    background-repeat: no-repeat; background-position: right 7px center;
  }
  .prof option { background: #0b1220; color: #cdd6e2; }
  .spacer { flex: 1; }
  .del { background: none; border: none; color: #6b7a8a; cursor: pointer; }
  .del:hover { color: #ff8080; }
  .attach { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 6px 8px; border-bottom: 1px solid #1c2a3a; color: #5a7a9a; font-size: 11px; }
  .attach label { color: #93c5fd; }
  .transcript { padding: 8px; display: flex; flex-direction: column; gap: 8px; max-height: 340px; overflow-y: auto; }
  .msg { display: flex; flex-direction: column; gap: 2px; }
  .msg .who { font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: #5a7a9a; }
  .msg .body { white-space: pre-wrap; font-size: 13px; }
  .msg.user .body { color: #e8eaf0; }
  .msg.assistant .body { color: #b9c6d6; }
  .caret { color: #7dd3fc; }
  .llm-error { color: #ff8080; font-size: 12px; }
  .llm-hint { color: #b58a3a; font-size: 12px; padding: 8px; }
  .composer { display: flex; gap: 6px; padding: 8px; border-top: 1px solid #1c2a3a; }
  .composer textarea { flex: 1; background: #0b1220; border: 1px solid #2a3a4a; border-radius: 6px; color: #e8eaf0; font-size: 13px; padding: 6px 8px; resize: vertical; }
  .send, .stop { align-self: flex-end; border: 1px solid #2a3a4a; border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 12px; }
  .send { background: #1e3a5a; color: #e8eaf0; }
  .stop { background: #5a1e1e; color: #ffd7d7; }
</style>
