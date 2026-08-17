// The selectable themes.
//
// normalizeTheme is the ONLY place the default lives. Go returns "" for a
// config written before this field existed, the pre-paint bootstrap in
// index.html reads whatever localStorage holds, and the picker reads the store
// — none of them may carry their own idea of what an unset theme means.
//
// `available` is a UI-only gate: Dark and Light still exist as real themes —
// still valid, still normalize to themselves, still paintable by hand-editing
// config.json — they are just not offered in the picker while their
// migration (CodeMirror, the graph explorer's DOM overlay, SketchCell's
// default pen, and more) is unfinished. Nothing here treats `available` as
// validity; isValidTheme and normalizeTheme deliberately still consult the
// full THEMES list.
//
// Dependency-free so it runs under `node --test`.

export const THEMES = Object.freeze([
  Object.freeze({ id: 'blue', label: 'Blue', available: true }),
  Object.freeze({ id: 'dark', label: 'Dark', available: false }),
  Object.freeze({ id: 'light', label: 'Light', available: false }),
])

export const AVAILABLE_THEMES = Object.freeze(THEMES.filter((t) => t.available))

export function isValidTheme(v) {
  return typeof v === 'string' && THEMES.some((t) => t.id === v)
}

export function normalizeTheme(v) {
  return isValidTheme(v) ? v : 'blue'
}
