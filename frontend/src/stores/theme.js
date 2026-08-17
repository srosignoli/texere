// The active theme.
//
// Three places hold a copy of the choice, and they are not peers:
//
//   config.json  — authoritative. Written by SetTheme, read by GetTheme.
//   localStorage — a mirror, read by the pre-paint bootstrap in index.html so a
//                  stored theme cannot flash blue before a Wails binding can be
//                  awaited. Reconciled from Go on every startup.
//   <html data-theme> — what the browser actually paints.
//
// This file lives in stores/ rather than lib/ on purpose: it imports wailsjs,
// and everything under lib/ must stay loadable by `node --test`.
//
// normalizeTheme is the only place the default lives — nothing here may invent
// its own idea of what an unset or unknown theme means.

import { writable } from 'svelte/store'

import { normalizeTheme } from '../lib/themes.js'
import { GetTheme, SetTheme } from '../../wailsjs/go/main/App'

const STORAGE_KEY = 'texere.theme'

// Seed from whatever the bootstrap already put on <html>, so the store agrees
// with the painted pixels from the first frame — before GetTheme resolves.
export const theme = writable(normalizeTheme(document.documentElement.dataset.theme))

// Paint the theme and mirror it to localStorage, without touching config.json.
// Startup uses this (Go is already the source of that value); the picker goes
// through applyTheme, which adds the write back to Go.
function activate(id) {
  const next = normalizeTheme(id)

  // Blue is :root's own values and carries no attribute — selecting it has to
  // REMOVE the attribute, not set it to "blue", or the two overriding blocks
  // would be the only themes that can ever be turned off.
  if (next === 'blue') delete document.documentElement.dataset.theme
  else document.documentElement.dataset.theme = next

  try {
    localStorage.setItem(STORAGE_KEY, next)
  } catch (e) {
    // A failed mirror costs a frame of blue on the next launch, nothing more —
    // config.json still carries the choice. Not worth failing the switch over.
    console.warn('theme: could not mirror the theme to localStorage:', e)
  }

  theme.set(next)
  return next
}

// Switch the theme: paint it, mirror it, and persist it. Painting comes first
// and unconditionally, so the theme still changes if the write to Go fails.
//
// Guarded the same way as the startup read below, and for the same reason: the
// binding throws SYNCHRONOUSLY when window.go is missing, so a trailing .catch()
// would never attach and the throw would escape this event handler to
// window.onerror.
export async function applyTheme(id) {
  const next = activate(id)
  try {
    await SetTheme(next)
  } catch (e) {
    console.error('theme: SetTheme failed — this choice will not survive a restart:', e)
  }
}

// Reconcile from the authoritative copy on load. GetTheme never returns empty,
// so a resolved value is always usable. If the binding is unavailable we keep
// what the bootstrap painted — but we say so, because a silent failure here
// looks exactly like a theme that "sometimes forgets".
//
// The CALL, not just the await, has to sit inside the guard. A wailsjs binding
// compiles to window['go']['main']['App']['GetTheme'](), which throws
// synchronously when window.go is absent — before any promise exists, so a
// trailing .catch() would never attach. Opening the Vite dev server directly in
// a browser is exactly that case, and there an unguarded call escapes to
// index.html's window.onerror, which replaces #app with the JS-error screen:
// the whole frontend down, over a theme. Every other store defers its bindings
// into a function called after mount; this one runs at import, so it guards.
async function reconcileFromGo() {
  try {
    activate(await GetTheme())
  } catch (e) {
    console.error('theme: GetTheme unavailable; keeping the theme the pre-paint bootstrap applied:', e)
  }
}
reconcileFromGo()
