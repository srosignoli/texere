// The app's own prompt, confirm and alert.
//
// These replace window.prompt / window.confirm / window.alert, which were
// wrong here for three separate reasons: GTK draws them, so no theme reaches
// them and all three of Texere's themes stop at their edge; they are modal at
// the OS level rather than the app's; and because the browser paints them
// rather than the page, they are invisible to a screenshot or a screen
// recording — the tutorial series had to narrate four of them as things the
// viewer cannot see.
//
// Promise-based on purpose, so a call site keeps reading in the order it
// happens:
//
//     const name = await askText('New file name')
//     if (name === null) return          // cancelled, exactly like prompt()
//
// The store holds at most ONE request. A second while one is open is a bug in
// the caller, not a queue to build: nothing in this app can reach two of these
// at once, and pretending otherwise would hide the mistake.
//
// This module is deliberately free of wailsjs imports so `node --test` can load
// it.

import { writable } from 'svelte/store'

// null when nothing is open; otherwise { kind, title, value, confirmLabel, resolve }.
export const dialogRequest = writable(null)

function open(spec) {
  return new Promise((resolve) => {
    dialogRequest.set({ ...spec, resolve })
  })
}

// Resolves to the string, or null if cancelled — prompt()'s contract.
export function askText(title, value = '', { confirmLabel = 'OK', placeholder = '' } = {}) {
  return open({ kind: 'text', title, value, confirmLabel, placeholder })
}

// Resolves true / false — confirm()'s contract.
export function askConfirm(title, { confirmLabel = 'OK' } = {}) {
  return open({ kind: 'confirm', title, confirmLabel })
}

// Resolves when acknowledged — alert()'s contract, which returns nothing.
export function showAlert(title, { confirmLabel = 'OK' } = {}) {
  return open({ kind: 'alert', title, confirmLabel })
}

// Called by the component. Settles the promise and clears the store, in that
// order, so a caller that opens another dialog from its .then() finds the slot
// free rather than clobbered.
export function settle(request, answer) {
  if (!request) return
  dialogRequest.set(null)
  request.resolve(answer)
}

// What cancelling means for each kind. Cancel is Escape, the backdrop and the
// Cancel button, and it must not read as "yes" anywhere.
export function cancelValue(kind) {
  if (kind === 'text') return null
  if (kind === 'confirm') return false
  return undefined
}
