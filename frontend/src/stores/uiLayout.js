import { writable } from 'svelte/store'

// Keyed under a versioned name so we can migrate the schema without colliding
// with stale state from previous shapes.
const KEY = 'texere.uiLayout.v1'

// Defaults match the previous static layout (App.svelte band-top 220 px,
// ColumnChapters 200 px, ColumnContextTags 160 px) so first-run users see no
// visual change.
const DEFAULTS = Object.freeze({
  topBandHeight: 220,
  chaptersWidth: 200,
  contextTagsWidth: 160,
  // Default height for the bottom-docked log panel when toggled open
  // (Ctrl+Shift+M). The open/closed flag itself lives in the logPanel store
  // and is intentionally NOT persisted — every launch starts with the panel
  // hidden.
  logPanelHeight: 240,
})

function load() {
  if (typeof localStorage === 'undefined') return { ...DEFAULTS }
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return { ...DEFAULTS }
    const parsed = JSON.parse(raw)
    return { ...DEFAULTS, ...parsed }
  } catch {
    return { ...DEFAULTS }
  }
}

export const uiLayout = writable(load())

// Persist on every change. Quota / serialization errors are swallowed because
// the only consequence is "sizes don't survive next launch" — losing a size
// preference is not worth surfacing to the user.
uiLayout.subscribe(value => {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(KEY, JSON.stringify(value))
  } catch {
    // ignore quota / serialization failures
  }
})
