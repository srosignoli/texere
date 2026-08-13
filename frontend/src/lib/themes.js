// The selectable themes.
//
// normalizeTheme is the ONLY place the default lives. Go returns "" for a
// config written before this field existed, the pre-paint bootstrap in
// index.html reads whatever localStorage holds, and the picker reads the store
// — none of them may carry their own idea of what an unset theme means.
//
// Dependency-free so it runs under `node --test`.

export const THEMES = Object.freeze([
  Object.freeze({ id: 'blue', label: 'Blue' }),
  Object.freeze({ id: 'dark', label: 'Dark' }),
  Object.freeze({ id: 'light', label: 'Light' }),
])

export function isValidTheme(v) {
  return typeof v === 'string' && THEMES.some((t) => t.id === v)
}

export function normalizeTheme(v) {
  return isValidTheme(v) ? v : 'blue'
}
