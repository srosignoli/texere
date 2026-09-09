<script>
  // The one on-screen surface behind askText / askConfirm / showAlert.
  // Mounted once, in App.svelte, and idle until lib/dialog.js puts a request
  // in the store.
  //
  // Keyboard contract matches what the native dialogs did, because people have
  // that in their fingers: Enter confirms, Escape cancels. The text field is
  // focused and its contents selected on open, so a suggested value can be
  // typed straight over.

  import { tick } from 'svelte'
  import { dialogRequest, settle, cancelValue } from '../lib/dialog.js'

  let input
  let value = ''
  // Which request `value` was seeded from. Without this the reactive block
  // re-runs while the field is being typed into and writes the suggested value
  // back over every keystroke — the field stayed empty no matter what was
  // typed, and even Playwright's fill() was clobbered. Seed once per request,
  // never again for the same one.
  let seeded = null

  // Seed the field from the request, and take focus once it is on screen.
  $: if ($dialogRequest && $dialogRequest !== seeded) {
    seeded = $dialogRequest
    value = $dialogRequest.value ?? ''
    focusSoon()
  }
  $: if (!$dialogRequest) seeded = null

  async function focusSoon() {
    await tick()
    input?.focus()
    input?.select()
  }

  function accept() {
    const req = $dialogRequest
    if (!req) return
    if (req.kind === 'text') settle(req, value)
    else if (req.kind === 'confirm') settle(req, true)
    else settle(req, undefined)
  }

  function cancel() {
    const req = $dialogRequest
    if (!req) return
    settle(req, cancelValue(req.kind))
  }

  // Bound on the window rather than the panel: Escape has to work before
  // anything inside has been focused.
  function onKeydown(e) {
    if (!$dialogRequest) return
    if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); cancel() }
    else if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); accept() }
  }
</script>

<svelte:window on:keydown|capture={onKeydown} />

{#if $dialogRequest}
  <!-- The backdrop cancels, the same as clicking away from a native dialog. -->
  <div class="dlg-scrim" on:click={cancel} on:keydown|stopPropagation role="presentation"></div>
  <div class="dlg" role="dialog" aria-modal="true" aria-label="Dialog">
    <div class="dlg-title">{$dialogRequest.title}</div>
    {#if $dialogRequest.kind === 'text'}
      <input
        class="dlg-input"
        bind:this={input}
        bind:value
        placeholder={$dialogRequest.placeholder ?? ''}
        aria-label="Dialog input" />
    {/if}
    <div class="dlg-actions">
      {#if $dialogRequest.kind !== 'alert'}
        <button class="dlg-btn" on:click={cancel} aria-label="Cancel dialog">Cancel</button>
      {/if}
      <button class="dlg-btn dlg-ok" on:click={accept} aria-label="Confirm dialog">
        {$dialogRequest.confirmLabel ?? 'OK'}
      </button>
    </div>
  </div>
{/if}

<style>
  .dlg-scrim { position: fixed; inset: 0; z-index: 300; background: var(--modal-scrim); }
  .dlg {
    position: fixed;
    z-index: 301;
    top: 22%;
    left: 50%;
    transform: translateX(-50%);
    width: min(460px, 92vw);
    /* text-align is explicit because style.css sets `html { text-align:
       center }`; without it the title and the field centre themselves. */
    text-align: left;
    background: var(--surface-modal);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 8px 28px rgba(0, 0, 0, 0.5);
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .dlg-title { color: var(--text); font-size: 13px; font-weight: 600; }
  .dlg-input {
    background: var(--control);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 7px 9px;
    font-size: 13px;
    font-family: inherit;
    width: 100%;
    box-sizing: border-box;
  }
  .dlg-input:focus { outline: none; border-color: var(--accent-primary); }
  .dlg-actions { display: flex; justify-content: flex-end; gap: 8px; }
  .dlg-btn {
    background: var(--control);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 5px 14px;
    font-size: 12px;
    font-family: inherit;
    cursor: pointer;
  }
  .dlg-btn:hover { background: var(--accent); }
  .dlg-ok { background: var(--accent); border-color: var(--accent-primary); }
</style>
