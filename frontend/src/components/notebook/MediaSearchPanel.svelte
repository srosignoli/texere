<script>
  import { onMount, onDestroy, tick } from 'svelte'
  import { ListMediaProviders, ListLanguages, SearchMedia, FetchMediaURL, ImportImage, ImportAudio, ResolveVideoPlayURL } from '../../../wailsjs/go/main/App'
  import { mediaErrorMessage } from '../../lib/media-error.js'
  import { getLastLanguage, setLastLanguage } from '../../lib/last-language.js'

  export let kind = 'image'
  export let query = ''
  export let lang = ''
  export let onQuery = () => {}   // (q) => persist the query on the cell
  export let onLang = () => {}    // (lang) => persist the language on the cell
  export let onKind = () => {}    // (kind) => persist the kind on the cell
  export let onPick = () => {}    // (result, {asset, mime}) => void

  const LIMIT = 24

  let allProviders = []
  let languages = []
  let providerId = ''
  let results = []
  let searched = false      // distinguishes "no results" from "not searched yet"
  let busy = false
  let err = ''
  let fetchingURL = ''      // FullURL of the result currently downloading
  let seq = 0               // guards against out-of-order responses
  let pasteURL = ''

  // Guards pick()/fetchPasted()/importFile() the same way `seq` guards
  // search(): each acquisition captures the current generation, and the
  // result is applied only if it is still current when the await resolves.
  // Bumped on teardown (Cancel unmounts this panel) and at the start of every
  // new acquisition, so a cancelled or superseded fetch can never call
  // onPick and overwrite the cell out from under the user.
  let acqGen = 0

  let previewURL = ''     // FullURL of the row currently previewing
  let previewErr = ''
  let audioEl              // the single shared <audio>

  function fmtDuration(ms) {
    if (!ms) return ''
    return (ms / 1000).toFixed(1) + 's'
  }
  // Video durations are minutes long, so a clock reads better than seconds.
  function fmtClock(ms) {
    const s = Math.round((ms || 0) / 1000)
    if (!s) return ''
    const m = Math.floor(s / 60)
    return `${m}:${String(s % 60).padStart(2, '0')}`
  }
  // "audio/wav" -> "wav"; "application/ogg" -> "ogg"
  function fmtFormat(mime) {
    if (!mime) return ''
    const s = mime.split('/')[1] ?? mime
    return s.replace(/^x-/, '')
  }

  // Stop and forget any preview. Must run before anything sets `busy` or clears
  // `results`: both tear the shared <audio> out of the DOM, and a stale
  // previewURL would leave a row rendering as if it were still playing.
  function stopPreview() {
    audioEl?.pause()
    previewURL = ''
    previewErr = ''
  }

  async function togglePreview(r) {
    previewErr = ''
    if (previewURL === r.fullUrl) { audioEl?.pause(); previewURL = ''; return }
    previewURL = r.fullUrl
    await tick() // let the src binding apply before playing
    audioEl?.play().catch(() => {})
  }

  function onPreviewError(e) {
    if (!previewURL) return
    previewErr = mediaErrorMessage(e.target?.error, 'Preview')
    previewURL = ''
  }

  onMount(async () => {
    try {
      allProviders = await ListMediaProviders()
      languages = await ListLanguages()
      if (!lang) {
        const remembered = getLastLanguage()
        const seeded = remembered && languages.some((l) => l.name === remembered)
          ? remembered
          : (languages[0]?.name ?? '')
        lang = seeded
        onLang(lang)
      }
    } catch (e) { err = String(e?.message ?? e) }
  })

  // Re-filter whenever the kind changes; a provider that cannot serve the
  // current kind must never stay selected, or Search queries the wrong one.
  $: providers = allProviders.filter((p) => p.kinds?.includes(kind))
  $: if (providers.length && !providers.some((p) => p.id === providerId)) {
    providerId = providers[0].id
  }

  onDestroy(() => { acqGen++; audioEl?.pause() })

  function setKind(k) {
    if (k === kind) return
    stopPreview()
    acqGen++ // supersede any in-flight pick: its result no longer belongs here
    kind = k
    results = []
    searched = false
    err = ''
    onKind(k)
  }

  async function search() {
    const q = query.trim()
    if (!q || !providerId) return
    stopPreview()
    acqGen++ // supersede any in-flight pick: its result no longer belongs here
    const mine = ++seq
    busy = true; err = ''; results = []
    // The pronunciation provider takes "language|word"; every other provider
    // takes the bare word.
    const arg = providerId === 'pronunciation' ? `${lang}|${q}` : q
    try {
      const rs = await SearchMedia(providerId, arg, kind, LIMIT)
      if (mine !== seq) return       // a newer search superseded this one
      results = rs ?? []
      searched = true
    } catch (e) {
      if (mine !== seq) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === seq) busy = false
    }
  }

  function onKey(e) {
    if (e.key === 'Enter') { e.preventDefault(); search() }
  }

  async function pick(r) {
    if (fetchingURL) return
    stopPreview()
    fetchingURL = r.fullUrl || r.sourcePage
    err = ''
    const mine = ++acqGen
    try {
      if (kind === 'video') {
        // Resolved here rather than during search: PeerTube and the Internet
        // Archive each need an extra request, which would be dozens per search.
        const playURL = await ResolveVideoPlayURL(providerId, r)
        if (mine !== acqGen) return
        // MediaCell.onPick(result, ref) reads embedURL/playerKind off `result`
        // (the first argument) — the same object playerKind is already merged
        // into — not off `ref`; `ref` is only ever asset/mime for stored kinds.
        onPick({ ...r, provider: providerId, playerKind: 'file', embedURL: playURL }, {})
      } else {
        const ref = await FetchMediaURL(r.fullUrl, kind)
        if (mine !== acqGen) return
        onPick({ ...r, provider: providerId }, ref)
      }
    } catch (e) {
      if (mine !== acqGen) return
      const msg = String(e?.message ?? e)
      err = msg.includes('403')
        ? `${msg} — this host blocks direct downloads. Open the source page and use Paste URL with the real image address.`
        : msg
    } finally {
      // Cleared unconditionally: pick() is already self-exclusive via the
      // guard above, so no later pick can be relying on this value. Gating the
      // clear on the generation would strand it forever once superseded,
      // leaving the grid permanently unclickable.
      fetchingURL = ''
    }
  }

  async function importFile() {
    stopPreview()
    err = ''
    busy = true
    const mine = ++acqGen
    try {
      const ref = kind === 'audio' ? await ImportAudio() : await ImportImage()
      if (mine !== acqGen) return   // cancelled or superseded while the dialog was open
      if (!ref) return // cancelled
      onPick({ kind, title: '', sourcePage: '', license: '', provider: 'file' }, ref)
    } catch (e) {
      if (mine !== acqGen) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === acqGen) busy = false
    }
  }

  const YOUTUBE_RE = /(?:youtube\.com\/.*[?&]v=|youtu\.be\/)([A-Za-z0-9_-]{6,})/

  async function fetchPasted() {
    const u = pasteURL.trim()
    if (!u) return
    stopPreview()
    err = ''
    busy = true
    const mine = ++acqGen
    try {
      if (kind === 'video') {
        // Video is referenced, never stored, so nothing is fetched here — which
        // is exactly why the scheme is checked: this string becomes a <video>
        // src or an iframe target unexamined otherwise. Same discipline as
        // openSource() in MediaCell, and it also catches the common mistake of
        // pasting a title or a file path.
        if (!/^https?:\/\//i.test(u)) {
          err = 'Paste an http(s) video or YouTube URL.'
          return
        }
        const isYT = YOUTUBE_RE.test(u)
        // embedURL/playerKind are read off this first object by MediaCell's
        // onPick(result, ref) — see the same note in pick().
        onPick(
          { kind, title: '', sourcePage: u, license: '', provider: isYT ? 'youtube' : 'url',
            playerKind: isYT ? 'iframe' : 'file', embedURL: u },
          {})
        return
      }
      const ref = await FetchMediaURL(u, kind)
      if (mine !== acqGen) return   // cancelled or superseded while fetching
      onPick({ kind, title: '', sourcePage: u, license: '', provider: 'url' }, ref)
    } catch (e) {
      if (mine !== acqGen) return
      err = String(e?.message ?? e)
    } finally {
      if (mine === acqGen) busy = false
    }
  }
</script>

<div class="ms-panel">
  <div class="ms-kinds">
    <button class="ms-btn" class:ms-on={kind === 'image'}
      on:click={() => setKind('image')} disabled={busy}>Image</button>
    <button class="ms-btn" class:ms-on={kind === 'audio'}
      on:click={() => setKind('audio')} disabled={busy}>Audio</button>
    <button class="ms-btn" class:ms-on={kind === 'video'}
      on:click={() => setKind('video')} disabled={busy}>Video</button>
    {#if kind === 'audio'}
      <select bind:value={lang} on:change={() => { onLang(lang); setLastLanguage(lang) }} disabled={busy} title="Recording language">
        {#each languages as l}<option value={l.name}>{l.name}</option>{/each}
      </select>
    {/if}
  </div>
  <div class="ms-bar">
    <select bind:value={providerId} disabled={busy || !providers.length}>
      {#each providers as p}<option value={p.id}>{p.name}</option>{/each}
    </select>
    <input class="ms-q" type="text" placeholder={kind === 'video' ? 'Search for a video…' : kind === 'audio' ? 'Search for a pronunciation…' : 'Search for a picture…'}
      bind:value={query} on:input={() => onQuery(query)} on:keydown={onKey} />
    <button class="ms-btn" on:click={search} disabled={busy || !query.trim() || !providerId}>
      {busy ? 'Searching…' : 'Search'}
    </button>
  </div>
  <div class="ms-alt">
    <!-- Video is referenced, never stored, so importing a local file has no
         meaning for it. The paste field stays available for every kind. -->
    {#if kind !== 'video'}
      <button class="ms-btn" on:click={importFile} disabled={busy}>Import file…</button>
    {/if}
    <input class="ms-q" type="text" placeholder={kind === 'video' ? '…or paste a video or YouTube URL' : kind === 'audio' ? '…or paste an audio URL' : '…or paste an image URL'}
      bind:value={pasteURL} on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); fetchPasted() } }} />
    <button class="ms-btn" on:click={fetchPasted} disabled={busy || !pasteURL.trim()}>Fetch</button>
  </div>

  {#if err}<div class="ms-err">{err}</div>{/if}

  {#if busy}
    <div class="ms-note">Searching {providers.find((p) => p.id === providerId)?.name ?? ''}…</div>
  {:else if searched && !results.length && !err}
    <div class="ms-note">No results for “{query.trim()}”. Try another provider, or Paste URL.</div>
  {:else if results.length}
    {#if kind === 'audio'}
      <audio bind:this={audioEl} src={previewURL || null} on:error={onPreviewError} on:ended={() => (previewURL = '')} hidden></audio>
      {#if previewErr}<div class="ms-err">{previewErr}</div>{/if}
      <div class="ms-rows">
        {#each results as r (r.fullUrl)}
          <div class="ms-row" class:ms-loading={fetchingURL === r.fullUrl}>
            <button class="ms-play" on:click={() => togglePreview(r)}
              title={previewURL === r.fullUrl ? 'Stop' : 'Preview'}>
              {previewURL === r.fullUrl ? '■' : '▶'}
            </button>
            <button class="ms-pick" on:click={() => pick(r)} title={r.sourcePage}>
              <span class="ms-word">{r.title}</span>
              <span class="ms-meta">
                {[r.lang, r.speaker, fmtDuration(r.durationMs), fmtFormat(r.mime)].filter(Boolean).join(' · ')}
              </span>
            </button>
          </div>
        {/each}
      </div>
    {:else if kind === 'video'}
      <!-- Keyed on sourcePage, NOT fullUrl: every video result carries an empty
           fullUrl until it is resolved on pick, so fullUrl would give every
           tile the same key. -->
      <div class="ms-grid ms-vgrid">
        {#each results as r (r.sourcePage)}
          <button class="ms-hit ms-vhit" class:ms-loading={fetchingURL === (r.fullUrl || r.sourcePage)}
            on:click={() => pick(r)} title={r.title}>
            {#if r.thumbUrl}<img src={r.thumbUrl} alt={r.title} loading="lazy" />{/if}
            {#if r.durationMs}<span class="ms-dur">{fmtClock(r.durationMs)}</span>{/if}
            <span class="ms-vtitle">{r.title}</span>
          </button>
        {/each}
      </div>
    {:else}
      <div class="ms-grid">
        {#each results as r (r.fullUrl)}
          <button class="ms-hit" class:ms-loading={fetchingURL === r.fullUrl}
            on:click={() => pick(r)} title={`${r.title}${r.license ? ' — ' + r.license : ''}`}>
            <img src={r.thumbUrl} alt={r.title} loading="lazy" />
          </button>
        {/each}
      </div>
    {/if}
  {/if}
</div>

<style>
  .ms-panel { padding: 8px; }
  .ms-kinds { display: flex; gap: 6px; align-items: center; margin-bottom: 6px; }
  .ms-bar { display: flex; gap: 6px; align-items: center; }
  .ms-alt { display: flex; gap: 6px; align-items: center; margin-top: 6px; }
  .ms-q { flex: 1 1 auto; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 8px; }
  .ms-btn { background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px 10px; cursor: pointer; }
  .ms-btn:disabled { opacity: 0.5; cursor: default; }
  /* Must come AFTER .ms-btn and its :hover — equal specificity, so source order
     decides. Declared earlier it was silently overridden and the selected kind
     looked identical to the unselected one. The filled accent also has to differ
     from :hover, or hovering an unselected button mimics selection. */
  .ms-btn.ms-on, .ms-btn.ms-on:hover { background: #2e6ca8; border-color: #6aa6dd; color: #f2f8ff; font-weight: 600; }
  select { background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; padding: 4px; }
  .ms-err { margin-top: 8px; padding: 6px 8px; border: 1px solid #6a3a3a; border-radius: 4px; background: #2a1616; color: #f9c; font-size: 12px; }
  .ms-note { margin-top: 8px; color: #8aa; font-size: 12px; }
  .ms-grid { margin-top: 8px; display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 6px; max-height: 320px; overflow-y: auto; }
  .ms-hit { padding: 0; border: 1px solid #2a3a4a; border-radius: 4px; background: #0b1220; cursor: pointer; overflow: hidden; aspect-ratio: 1; }
  .ms-hit:hover { border-color: #4a7aa8; }
  .ms-hit.ms-loading { opacity: 0.4; cursor: default; }
  .ms-hit img { width: 100%; height: 100%; object-fit: cover; display: block; }
  /* MUST stay after every .ms-hit rule above: equal specificity, so source
     order decides which wins. Declared earlier these are silently dead and the
     video tiles render as squares with stretched thumbnails — the same mistake
     that once made .ms-on invisible in this file. */
  .ms-vgrid { grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); }
  .ms-vhit { position: relative; aspect-ratio: auto; display: flex; flex-direction: column; align-items: stretch; padding: 0 0 4px; }
  .ms-vhit img { width: 100%; aspect-ratio: 16 / 9; height: auto; object-fit: cover; display: block; }
  .ms-dur { position: absolute; right: 4px; top: 4px; background: rgba(0,0,0,0.75); color: #fff; font-size: 10px; padding: 1px 4px; border-radius: 3px; }
  .ms-vtitle { padding: 4px 6px 0; font-size: 11px; color: #cde; text-align: left; line-height: 1.25; max-height: 2.6em; overflow: hidden; }
  .ms-rows { margin-top: 8px; display: flex; flex-direction: column; gap: 4px; max-height: 320px; overflow-y: auto; }
  .ms-row { display: flex; align-items: stretch; gap: 6px; }
  .ms-row.ms-loading { opacity: 0.4; }
  .ms-play { width: 32px; flex: 0 0 auto; background: #1b2636; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; cursor: pointer; }
  .ms-play:hover { background: #24334a; }
  .ms-pick { flex: 1 1 auto; display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 4px 8px; background: #0b1220; color: #cde; border: 1px solid #2a3a4a; border-radius: 4px; cursor: pointer; text-align: left; }
  .ms-pick:hover { border-color: #4a7aa8; }
  .ms-word { font-size: 13px; }
  .ms-meta { font-size: 11px; color: #8aa; }
</style>
