<script>
  import { onDestroy } from 'svelte'
  import { ImportAudio, SaveRecording } from '../../../wailsjs/go/main/App'

  export let cell
  export let onChange = () => {}
  export let onDelete = () => {}
  export let readOnly = false
  export let context = null   // unused; shared cell prop set
  export let cells = []       // unused
  export let onPush = () => {} // unused

  let recording = false
  let recUnavailable = false
  let recErr = ''
  let elapsed = 0
  let missing = false
  let rec = null
  let chunks = []
  let stream = null
  let timer = null
  let startedAt = 0

  function persist() { onChange(cell) }

  async function startRec() {
    if (recording || stream) return
    recErr = ''
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true })
    } catch (e) {
      recUnavailable = true
      recErr = 'Recording isn’t available in this webview — import an audio file instead.'
      return
    }
    const prefs = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus', 'audio/ogg', 'audio/mp4', '']
    let m = ''
    for (const p of prefs) { if (!p || (window.MediaRecorder && MediaRecorder.isTypeSupported && MediaRecorder.isTypeSupported(p))) { m = p; break } }
    try {
      rec = m ? new MediaRecorder(stream, { mimeType: m }) : new MediaRecorder(stream)
      chunks = []
      rec.ondataavailable = (e) => { if (e.data && e.data.size) chunks.push(e.data) }
      rec.onstop = onStop
      rec.onerror = () => {
        clearInterval(timer)
        recording = false
        if (stream) { stream.getTracks().forEach((t) => t.stop()); stream = null }
        recErr = 'Recording error — please try again.'
      }
      rec.start()
    } catch (e) {
      if (stream) { stream.getTracks().forEach((t) => t.stop()); stream = null }
      recErr = 'Could not start recording: ' + String(e?.message ?? e)
      return
    }
    recording = true
    startedAt = Date.now()
    elapsed = 0
    timer = setInterval(() => { elapsed = Date.now() - startedAt }, 200)
  }

  function stopRec() { try { if (rec && rec.state !== 'inactive') rec.stop() } catch (_) {} }

  async function onStop() {
    clearInterval(timer)
    recording = false
    const durationMs = Date.now() - startedAt
    const mime = (rec && rec.mimeType) || 'audio/webm'
    if (stream) { stream.getTracks().forEach((t) => t.stop()); stream = null }
    if (!chunks.length) {
      recErr = 'Empty recording.'
      return
    }
    const blob = new Blob(chunks, { type: mime })
    if (!blob.size) {
      recErr = 'Empty recording.'
      return
    }
    try {
      const b64 = await blobToBase64(blob)
      const ref = await SaveRecording(b64, mime)
      cell.asset = ref.asset; cell.mime = ref.mime; cell.durationMs = durationMs
      missing = false; persist()
    } catch (e) { recErr = String(e?.message ?? e) }
  }

  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const r = new FileReader()
      r.onload = () => resolve(String(r.result).split(',')[1])
      r.onerror = () => reject(new Error('read failed'))
      r.readAsDataURL(blob)
    })
  }

  async function importAudio() {
    recErr = ''
    try {
      const ref = await ImportAudio()
      if (!ref) return
      cell.asset = ref.asset; cell.mime = ref.mime; cell.durationMs = 0
      missing = false; persist()
    } catch (e) { recErr = String(e?.message ?? e) }
  }

  function onMeta(e) {
    const d = Math.round((e.target.duration || 0) * 1000)
    if (d && d !== cell.durationMs) { cell.durationMs = d; persist() }
  }

  function replace() { cell.asset = ''; cell.mime = ''; cell.durationMs = 0; missing = false; persist() }

  function fmt(ms) {
    const s = Math.floor((ms || 0) / 1000)
    return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
  }

  onDestroy(() => { clearInterval(timer); if (stream) stream.getTracks().forEach((t) => t.stop()) })
</script>

<div class="au-cell">
  <div class="au-bar">
    <span class="au-title">🎙 Audio</span>
    {#if cell.asset}<span class="au-dur">{fmt(cell.durationMs)}</span>{/if}
    <span class="au-spacer"></span>
    {#if !readOnly && cell.asset}<button on:click={replace} title="Replace">Replace</button>{/if}
    {#if !readOnly}<button class="au-del" on:click={onDelete} title="Delete cell">✕</button>{/if}
  </div>

  {#if cell.asset}
    {#if missing}
      <div class="au-missing">audio not found: {cell.asset}</div>
    {:else}
      <audio class="au-player" src={"/media/" + cell.asset} controls on:loadedmetadata={onMeta} on:error={() => (missing = true)}></audio>
    {/if}
  {:else if !readOnly}
    <div class="au-empty">
      {#if recording}
        <button class="au-rec" on:click={stopRec}>■ Stop ({fmt(elapsed)})</button>
      {:else}
        <button class="au-rec" on:click={startRec} disabled={recUnavailable}>● Record</button>
        <button on:click={importAudio}>+ Import audio</button>
      {/if}
      {#if recErr}<div class="au-err">{recErr}</div>{/if}
    </div>
  {:else}
    <div class="au-missing">no audio</div>
  {/if}
</div>

<style>
  .au-cell { background: #0e1622; border: 1px solid #26374a; border-radius: 8px; margin-bottom: 10px; overflow: hidden; }
  .au-bar { display: flex; align-items: center; gap: 8px; padding: 5px 8px; border-bottom: 1px solid #1c2a3a; }
  .au-title { color: #7dd3fc; font-size: 11px; letter-spacing: .06em; text-transform: uppercase; }
  .au-dur { color: #93c5fd; font-size: 11px; }
  .au-spacer { flex: 1; }
  .au-bar button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 2px 8px; cursor: pointer; font-size: 12px; }
  .au-del { background: none !important; border: none !important; color: #6b7a8a !important; }
  .au-del:hover { color: #ff8080 !important; }
  .au-empty { display: flex; align-items: center; gap: 8px; padding: 10px; flex-wrap: wrap; }
  .au-empty button { background: #14233a; color: #93c5fd; border: 1px solid #2a3a4a; border-radius: 6px; padding: 4px 12px; cursor: pointer; font-size: 12px; }
  .au-empty button:disabled { opacity: .4; cursor: default; }
  .au-player { width: 100%; display: block; padding: 8px; }
  .au-err { color: #ff8080; font-size: 12px; width: 100%; }
  .au-missing { padding: 14px; color: #ff8080; font-size: 13px; }
</style>
