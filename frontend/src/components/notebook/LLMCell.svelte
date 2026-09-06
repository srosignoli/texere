<script>
  import { onMount, onDestroy } from 'svelte'
  import { GetLLMConfig, ListSkills } from '../../../wailsjs/go/main/App'
  import { streamTurn } from '../../lib/llm.js'
  import { buildSystem, skillIdForCell } from '../../lib/llm-prompt.js'
  import { bakeSketch, bakeImage } from '../../lib/bake.js'

  export let cell
  export let context = null
  export let cells = []
  export let onChange = () => {}
  export let readOnly = false

  let profiles = []
  let input = ''
  let streaming = false
  let partial = ''
  let turn = null
  let error = ''
  let lastSent = ''

  let skills = []
  let skillMissing = false // the resolved skill id names a file that is no longer present

  onMount(async () => { try { const c = await GetLLMConfig(); profiles = c.profiles ?? [] } catch { /* settings not configured */ } })
  onMount(async () => {
    try {
      skills = await ListSkills()
      const found = skills.find((s) => s.id === skillIdForCell(cell))
      // A saved skill that has gone away must NOT be silently swapped for
      // another: the conversation would continue under a different teaching
      // method with nothing to say so, and persisting the swap would destroy
      // the original choice. Surface it and let the user decide.
      //
      // Resolution here is READ-ONLY: activeSkill below re-derives the same
      // lookup from cell + skills, so merely opening a cell — legacy or not —
      // never calls selectSkill and never writes to the notebook. Only a real
      // pick via onPickSkill persists.
      if (!found) skillMissing = true
    } catch (e) {
      // A failed fetch must not leave Send clickable: activeSkill below stays
      // undefined because `skills` is still [], which is what actually gates
      // send() and the button — not a separate "did the call settle" flag.
      // Named here so the disabled Send button has a visible reason on screen.
      error = 'Could not load skills: ' + String(e?.message ?? e)
    }
  })
  onDestroy(() => turn?.cancel())

  $: attached = (cell.attachments ?? []).map((id) => cells.find((c) => c.id === id)).filter(Boolean)
  $: hasProvider = profiles.length > 0
  // Derived from skillIdForCell(cell), not cell.skill directly, so a legacy
  // cell (mode set, skill unset) resolves its skill for display and for the
  // system prompt without ever being written to disk just for being opened.
  //
  // NOTE: a resolved activeSkill is NOT itself sufficient to send — the Go
  // side deliberately lists a skill with an empty body rather than dropping
  // it (see internal/skills/load_test.go, TestListKeepsAnEmptyBodySkill), so
  // activeSkill can be a real, truthy object whose .body is ''. The actual
  // precondition for sending is a NON-EMPTY resolved body, checked directly
  // via activeSkill?.body?.trim() at both guarded call sites below — not
  // "activeSkill exists", which two rounds of narrower flags (a promise
  // race, then a fetch rejection) already showed is the wrong boundary to
  // draw this at.
  $: activeSkill = skills.find((s) => s.id === skillIdForCell(cell))

  function persist() { onChange(cell) }
  function selectSkill(s) {
    cell.skill = s.id
    cell.skillName = s.name
    cell = cell
    skillMissing = false
    persist()
  }
  function onPickSkill(e) {
    const s = skills.find((x) => x.id === e.target.value)
    if (s) selectSkill(s)
  }
  function setProfile(id) { cell.profileId = id || null; cell = cell; persist() }
  function toggleAttach(id) {
    const a = new Set(cell.attachments ?? [])
    if (a.has(id)) a.delete(id); else a.add(id)
    cell.attachments = [...a]; cell = cell; persist()
  }

  async function send() {
    const text = input.trim()
    if (!text || streaming || !activeSkill?.body?.trim()) return
    lastSent = text
    cell.messages = [...cell.messages, { role: 'user', content: text }]
    cell = cell
    input = ''
    error = ''
    partial = ''
    streaming = true

    // Bake attached sketch/image cells; attach them once to the first user message.
    let sendMessages = cell.messages
    try {
      const images = []
      for (const c of attached) {
        if (c.type === 'sketch') images.push(await bakeSketch(c))
        else if (c.type === 'image') images.push(await bakeImage(c))
      }
      if (images.length) {
        const idx = cell.messages.findIndex((m) => m.role === 'user')
        sendMessages = cell.messages.map((m, i) => (i === idx ? { ...m, images } : m))
      }
    } catch (e) {
      // Bake failed — undo the just-added user message and abort.
      cell.messages = cell.messages.slice(0, -1)
      cell = cell
      error = String(e?.message ?? e)
      streaming = false
      return
    }

    const system = buildSystem(activeSkill?.body ?? '', context, attached)
    turn = streamTurn({
      profileId: cell.profileId,
      system,
      messages: sendMessages,
      onDelta: (t) => { partial += t },
      // The full reply comes with llm:done; `partial` is only the live
      // preview, so it is the fallback rather than the source of truth.
      onDone: (full) => { finish(full || partial) },
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
  {#if skillMissing}
    <div class="llm-warn">
      {#if readOnly}
        Skill "{cell.skillName || skillIdForCell(cell)}" is not installed on this machine.
      {:else}
        Skill "{cell.skillName || skillIdForCell(cell)}" was not found. Choose one below to continue.
      {/if}
    </div>
  {:else if activeSkill && !activeSkill.body?.trim()}
    <!-- The backend deliberately lists a skill with an empty body rather than
         dropping it (a half-edited .md — frontmatter written, prompt not).
         Sending would otherwise go out with no framing at all, silently. -->
    <div class="llm-warn">
      {#if readOnly}
        Skill "{activeSkill.name}" has no content, so it cannot frame a conversation.
      {:else}
        Skill "{activeSkill.name}" has no content yet. Edit its file to add a system prompt, or choose a different skill below.
      {/if}
    </div>
  {/if}
  <div class="llm-bar">
    <!-- Bound to activeSkill's id, not cell.skill directly: a legacy cell (mode
         set, skill unset) resolves via skillIdForCell without ever being
         written to, so the picker must read the same derived value to show
         the right option selected instead of a blank one. -->
    <select class="skill" value={activeSkill?.id ?? ''} on:change={onPickSkill}
      disabled={readOnly || streaming} title="Learning skill">
      {#each skills as s (s.id)}<option value={s.id}>{s.name}</option>{/each}
    </select>
    <select class="prof" value={cell.profileId || ''} on:change={(e) => setProfile(e.target.value)} disabled={readOnly} title="Model profile">
      <option value="">active default</option>
      {#each profiles as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
  </div>

  {#if !readOnly && cells.filter((c) => c.id !== cell.id && ['n4l', 'llm', 'graph', 'compute', 'sketch', 'image'].includes(c.type)).length}
    <div class="attach">
      <span>Attach context:</span>
      {#each cells.filter((c) => c.id !== cell.id && ['n4l', 'llm', 'graph', 'compute', 'sketch', 'image'].includes(c.type)) as c}
        <label><input type="checkbox" checked={(cell.attachments ?? []).includes(c.id)} on:change={() => toggleAttach(c.id)} /> {c.type === 'n4l' ? 'draft' : c.type === 'graph' ? 'graph' : c.type === 'compute' ? 'compute' : c.type === 'sketch' ? 'sketch' : c.type === 'image' ? 'image' : 'llm'} {c.id.slice(-4)}</label>
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
      <div class="llm-hint">No LLM provider configured — open ⚙ Settings → LLM, Speech &amp; Skills to add one.</div>
    {:else}
      <div class="composer">
        <textarea bind:value={input} on:keydown={onKey}
          placeholder={activeSkill?.placeholder || 'Ask…'} rows="2"
          disabled={streaming || skillMissing}></textarea>
        {#if streaming}<button class="stop" on:click={stop}>Stop</button>{:else}<button class="send" on:click={send} disabled={!activeSkill?.body?.trim()}>Send</button>{/if}
      </div>
    {/if}
  {/if}
</div>

<style>
  .llm-cell { background: var(--surface-modal); border: 1px solid var(--border-card); border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .llm-bar { display: flex; align-items: center; gap: 8px; padding: 6px 8px; border-bottom: 1px solid var(--border-subtle); }
  .llm-warn { padding: 6px 8px; color: var(--text-notice); font-size: 12px; border-bottom: 1px solid var(--border); }
  .skill { background: var(--control); color: var(--accent-text-alt); border: 1px solid var(--control-border); border-radius: 6px; padding: 2px 6px; font-size: 12px; }
  /* appearance:none lets WebKitGTK apply the app's dark-blue background to the
     select and its popup instead of the UA grey; data-URI chevron replaces the
     removed native arrow. The chevron's fill is baked into the data URI as a
     literal (%23cdd6e2 = #cdd6e2) — CSS custom properties cannot be
     interpolated into a url() string, so it cannot track var(--text-modal)
     and stays Blue's exact shade in every theme. Pre-existing constraint, not
     introduced by this migration. */
  .prof {
    -webkit-appearance: none; appearance: none;
    background: var(--surface-sunken); border: 1px solid var(--border); border-radius: 6px; color: var(--text-modal); font-size: 12px; padding: 2px 22px 2px 6px;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath fill='%23cdd6e2' d='M0 0l5 6 5-6z'/%3E%3C/svg%3E");
    background-repeat: no-repeat; background-position: right 7px center;
  }
  .prof option { background: var(--surface-sunken); color: var(--text-modal); }
  .attach { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; padding: 6px 8px; border-bottom: 1px solid var(--border-subtle); color: var(--accent-dim); font-size: 11px; }
  .attach label { color: var(--accent-text-alt); }
  .transcript { padding: 8px; display: flex; flex-direction: column; gap: 8px; max-height: 340px; overflow-y: auto; }
  .msg { display: flex; flex-direction: column; gap: 2px; }
  .msg .who { font-size: 10px; letter-spacing: .06em; text-transform: uppercase; color: var(--accent-dim); }
  .msg .body { white-space: pre-wrap; font-size: 13px; }
  .msg.user .body { color: var(--text); }
  .msg.assistant .body { color: var(--text-cell-muted); }
  .caret { color: var(--accent-text); }
  .llm-error { color: var(--danger); font-size: 12px; }
  .llm-hint { color: var(--text-notice); font-size: 12px; padding: 8px; }
  .composer { display: flex; gap: 6px; padding: 8px; border-top: 1px solid var(--border-subtle); }
  .composer textarea { flex: 1; background: var(--surface-sunken); border: 1px solid var(--border); border-radius: 6px; color: var(--text); font-size: 13px; padding: 6px 8px; resize: vertical; }
  .send, .stop { align-self: flex-end; border: 1px solid var(--border); border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 12px; }
  .send { background: var(--accent); color: var(--text); }
  .stop { background: var(--danger-fill); color: var(--danger-fill-text); }
</style>
