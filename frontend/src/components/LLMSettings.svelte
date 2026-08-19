<script>
  import { onMount } from 'svelte'
  import {
    GetLLMConfig, SaveLLMProfile, DeleteLLMProfile, SetActiveProfile, TestLLMProfile,
    GetSTTConfig, SaveSTTProfile, DeleteSTTProfile, SetActiveSTTProfile,
    ListSkills, DuplicateSkill, RevealSkillsDir, SkillsDir,
  } from '../../wailsjs/go/main/App'

  export let onClose = () => {}

  const LLM_PRESETS = [
    { name: 'Anthropic',  kind: 'anthropic',         baseUrl: 'https://api.anthropic.com',                          model: 'claude-opus-4-8' },
    { name: 'OpenAI',     kind: 'openai-compatible',  baseUrl: 'https://api.openai.com/v1',                          model: 'gpt-4o' },
    { name: 'Google',     kind: 'openai-compatible',  baseUrl: 'https://generativelanguage.googleapis.com/v1beta/openai', model: 'gemini-2.0-flash' },
    { name: 'OpenRouter', kind: 'openai-compatible',  baseUrl: 'https://openrouter.ai/api/v1',                       model: '' },
    { name: 'Local (Ollama)', kind: 'openai-compatible', baseUrl: 'http://localhost:11434/v1',                       model: 'llama3.1' },
  ]
  const STT_PRESETS = [
    { name: 'OpenAI Whisper', kind: 'openai-compatible', baseUrl: 'https://api.openai.com/v1', model: 'whisper-1' },
    { name: 'Local whisper',  kind: 'openai-compatible', baseUrl: 'http://localhost:8080/v1',  model: 'whisper-1' },
  ]

  // Per-tab binding + option set. Called with the current tab so async actions
  // never read a stale reactive value.
  function apiFor(t) {
    return t === 'stt'
      ? { get: GetSTTConfig, save: SaveSTTProfile, del: DeleteSTTProfile, setActive: SetActiveSTTProfile, test: null, presets: STT_PRESETS, hasKind: false }
      : { get: GetLLMConfig, save: SaveLLMProfile, del: DeleteLLMProfile, setActive: SetActiveProfile, test: TestLLMProfile, presets: LLM_PRESETS, hasKind: true }
  }

  let tab = 'llm'
  let profiles = []
  let activeId = ''
  let editing = null
  let testMsg = {}
  let error = ''
  let skills = []
  let skillsDir = ''

  onMount(load)
  async function load() {
    error = ''
    try { const c = await apiFor(tab).get(); profiles = c.profiles ?? []; activeId = c.activeProfileId ?? '' }
    catch (e) { error = String(e?.message ?? e) }
  }

  // Read on demand rather than watched: this is a folder edited a handful of
  // times ever, in the user's own editor, so re-listing when the tab opens is
  // enough and costs nothing.
  async function loadSkills() {
    error = ''
    try {
      skills = await ListSkills()
      // Fetched here rather than on mount so it is always current, and so a
      // failure to resolve it cannot stop the list from rendering.
      skillsDir = await SkillsDir()
    } catch (e) { error = String(e?.message ?? e) }
  }
  async function duplicate(id) {
    error = ''
    try { await DuplicateSkill(id); await loadSkills() } catch (e) { error = String(e?.message ?? e) }
  }
  async function revealSkills() {
    error = ''
    try { await RevealSkillsDir() } catch (e) { error = String(e?.message ?? e) }
  }

  function setTab(t) {
    tab = t; editing = null; testMsg = {}
    if (t === 'skills') loadSkills()
    else load()
  }

  function addFrom(preset) {
    editing = { id: `p-${Date.now()}`, name: preset.name, kind: preset.kind, baseUrl: preset.baseUrl, model: preset.model, apiKey: '', apiKeyEnv: '' }
  }
  function edit(p) { editing = { ...p } }
  async function save() {
    error = ''
    try { await apiFor(tab).save(editing); editing = null; await load() }
    catch (e) { error = String(e?.message ?? e) }
  }
  async function del(id) { error = ''; try { await apiFor(tab).del(id); await load() } catch (e) { error = String(e?.message ?? e) } }
  async function setActive(id) { error = ''; try { await apiFor(tab).setActive(id); activeId = id } catch (e) { error = String(e?.message ?? e) } }
  async function test(id) {
    const fn = apiFor(tab).test
    if (!fn) return
    testMsg = { ...testMsg, [id]: 'testing…' }
    try { await fn(id); testMsg = { ...testMsg, [id]: '✓ ok' } }
    catch (e) { testMsg = { ...testMsg, [id]: '✕ ' + String(e?.message ?? e) } }
  }
</script>

<div class="ov-backdrop" on:click={onClose}></div>
<div class="ov-modal">
  <div class="ov-head"><span>LLM, Speech &amp; Skills</span><button on:click={onClose}>✕</button></div>
  <div class="ov-tabs">
    <button class:sel={tab === 'llm'} on:click={() => setTab('llm')}>LLM Providers</button>
    <button class:sel={tab === 'stt'} on:click={() => setTab('stt')}>Speech-to-text</button>
    <button class:sel={tab === 'skills'} on:click={() => setTab('skills')}>Skills</button>
  </div>
  {#if error}<div class="ov-error">{error}</div>{/if}

  {#if tab === 'skills'}
    <div class="skills">
      <p class="hint">
        Skills are Markdown files holding the system prompt that frames an LLM
        cell. Edit them in your own editor — the list refreshes when you reopen
        this tab.
      </p>
      {#each skills as s (s.id)}
        <div class="skill-row">
          <div class="skill-main">
            <span class="skill-name">{s.name}</span>
            <span class="skill-origin">{s.builtin ? 'built-in' : (s.overridden ? 'yours, overrides built-in' : 'yours')}</span>
            {#if !s.body}<span class="skill-empty">(no content)</span>{/if}
            {#if s.description}<div class="skill-desc">{s.description}</div>{/if}
          </div>
          {#if s.builtin}
            <button on:click={() => duplicate(s.id)} title="Write an editable copy you can change">Duplicate</button>
          {/if}
        </div>
      {/each}
      <!-- The path is shown outright, not just behind the button: there is no
           file manager to open on a headless box, in a container, or under WSL
           without extra tooling, and the folder is useless if you cannot find
           it. The button is a convenience on top of this, not the way in. -->
      <div class="skills-path">
        <span class="skills-path-label">Folder</span>
        <code class="skills-path-value">{skillsDir || '…'}</code>
      </div>
      <button class="reveal" on:click={revealSkills}>Open skills folder</button>
    </div>
  {:else if editing}
    <div class="ov-form">
      <label>Name<input bind:value={editing.name} /></label>
      {#if apiFor(tab).hasKind}
        <label>Kind
          <select bind:value={editing.kind}>
            <option value="openai-compatible">openai-compatible</option>
            <option value="anthropic">anthropic</option>
          </select>
        </label>
      {/if}
      <label>Base URL<input bind:value={editing.baseUrl} /></label>
      <label>Model<input bind:value={editing.model} placeholder={tab === 'stt' ? 'e.g. whisper-1' : 'e.g. claude-opus-4-8'} /></label>
      <label>API key<input type="password" bind:value={editing.apiKey} placeholder={tab === 'stt' ? 'optional (local servers)' : 'leave blank to use an env var'} /></label>
      <label>…or env var name<input bind:value={editing.apiKeyEnv} placeholder="e.g. OPENAI_API_KEY" /></label>
      <div class="ov-actions"><button class="primary" on:click={save}>Save</button><button on:click={() => (editing = null)}>Cancel</button></div>
    </div>
  {:else}
    <ul class="ov-list">
      {#each profiles as p (p.id)}
        <li>
          <label class="ov-active"><input type="radio" checked={p.id === activeId} on:change={() => setActive(p.id)} /></label>
          <div class="ov-meta"><b>{p.name}</b><span>{p.kind} · {p.model || '(no model)'}</span></div>
          {#if apiFor(tab).test}<button on:click={() => test(p.id)}>Test</button>{/if}
          <button on:click={() => edit(p)}>Edit</button>
          <button on:click={() => del(p.id)}>Delete</button>
          {#if testMsg[p.id]}<span class="ov-test">{testMsg[p.id]}</span>{/if}
        </li>
      {/each}
    </ul>
    <div class="ov-presets">
      <span>Add:</span>
      {#each apiFor(tab).presets as preset}<button on:click={() => addFrom(preset)}>{preset.name}</button>{/each}
    </div>
  {/if}
</div>

<style>
  /* Must clear SettingsMenu's popover (60) and StudySurface (60) — this can
     now be launched from the settings popover, not just the notebook gear —
     while leaving room below ShortcutsPanel (211), which must stay on top of
     everything including this. */
  .ov-backdrop { position: fixed; inset: 0; background: rgba(0,0,0,0.5); z-index: 200; }
  .ov-modal { position: fixed; top: 8%; left: 50%; transform: translateX(-50%); width: 560px; max-width: 92vw; max-height: 80vh; overflow: auto; z-index: 201; background: var(--surface-modal); border: 1px solid var(--border); border-radius: 10px; color: var(--text-modal); }
  .ov-head { display: flex; justify-content: space-between; align-items: center; padding: 10px 14px; border-bottom: 1px solid var(--border-card); color: var(--accent-text); font-weight: 600; }
  .ov-head button { background: none; border: none; color: var(--text-tab-muted); cursor: pointer; }
  .ov-tabs { display: flex; gap: 6px; padding: 8px 14px 0; }
  .ov-tabs button { background: none; border: none; border-bottom: 2px solid transparent; color: var(--text-tab-muted); cursor: pointer; padding: 4px 6px; font-size: 12px; }
  .ov-tabs button.sel { color: var(--accent-text); border-bottom-color: var(--accent-text); }
  .ov-error { color: var(--danger); padding: 6px 14px; font-size: 12px; }
  .ov-list { list-style: none; margin: 0; padding: 8px 12px; }
  .ov-list li { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-bottom: 1px solid var(--border-row); }
  .ov-meta { display: flex; flex-direction: column; flex: 1; min-width: 0; }
  .ov-meta span { color: var(--accent-dim); font-size: 11px; }
  .ov-test { font-size: 11px; color: var(--accent-text-alt); }
  .ov-list button, .ov-presets button, .ov-actions button { background: var(--accent); color: var(--text); border: 1px solid var(--border); border-radius: 6px; padding: 3px 9px; cursor: pointer; font-size: 12px; }
  .ov-actions button.primary, .primary { background: var(--accent-hover); }
  .ov-presets { display: flex; gap: 6px; align-items: center; padding: 10px 14px; border-top: 1px solid var(--border-card); flex-wrap: wrap; }
  .ov-form { display: flex; flex-direction: column; gap: 8px; padding: 12px 14px; }
  .ov-form label { display: flex; flex-direction: column; gap: 3px; font-size: 12px; color: var(--accent-text-alt); }
  .ov-form input, .ov-form select { background: var(--surface-sunken); border: 1px solid var(--border); border-radius: 6px; color: var(--text); padding: 5px 8px; font-size: 13px; }
  /* Native <select> popup/chevron styling — left untouched per the task
     brief: themed by color-scheme, not by this migration; the picker task
     handles it. appearance:none makes WebKitGTK render the select (and its
     popup) as a CSS-styled control so the app's dark-blue background applies
     instead of the UA grey; a data-URI chevron (#cdd6e2, out of scope)
     replaces the removed native arrow. */
  .ov-form select {
    -webkit-appearance: none; appearance: none; padding-right: 26px;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath fill='%23cdd6e2' d='M0 0l5 6 5-6z'/%3E%3C/svg%3E");
    background-repeat: no-repeat; background-position: right 9px center;
  }
  /* Native select popup option list. The chevron/appearance styling above
     stays untouched (color-scheme-driven, per that comment) but this
     explicit option fallback was hardcoded to Blue's dark palette, which
     fights color-scheme once Light flips it to light — reuses the general
     --surface-sunken/--text tokens (byte-identical to the literals here in
     Blue) so the option list follows the active theme instead. */
  .ov-form select option { background: var(--surface-sunken); color: var(--text); }
  .ov-actions { flex-direction: row; display: flex; gap: 8px; }
  .skills { display: flex; flex-direction: column; gap: 8px; }
  .hint { color: var(--text-faint); font-size: 12px; margin: 0 0 4px; }
  .skill-row { display: flex; align-items: flex-start; gap: 8px; padding: 6px 8px; border: 1px solid var(--border); border-radius: 6px; }
  .skill-main { flex: 1; min-width: 0; }
  .skill-name { color: var(--text); font-size: 13px; }
  .skill-origin { color: var(--text-faint); font-size: 11px; margin-left: 6px; }
  .skill-empty { color: var(--text-notice); font-size: 11px; margin-left: 6px; }
  .skill-desc { color: var(--text-faint); font-size: 11px; margin-top: 2px; }
  .skills-path { display: flex; align-items: baseline; gap: 8px; margin-top: 2px; }
  .skills-path-label { color: var(--text-faint); font-size: 11px; }
  .skills-path-value { color: var(--accent-text-alt); font-size: 11px; font-family: ui-monospace, monospace; word-break: break-all; user-select: text; }
  .reveal { align-self: flex-start; background: var(--control); color: var(--accent-text-alt); border: 1px solid var(--control-border); border-radius: 6px; padding: 4px 10px; cursor: pointer; font-size: 12px; }
</style>
