// The selectable themes.
//
// normalizeTheme is the ONLY place the default lives. Go returns "" for a
// config written before this field existed, the pre-paint bootstrap in
// index.html reads whatever localStorage holds, and the picker reads the store
// — none of them may carry their own idea of what an unset theme means.
//
// `available` is a UI-only gate, not a validity check: it controls whether a
// theme is offered in the picker, nothing more. isValidTheme and
// normalizeTheme deliberately consult the full THEMES list regardless of
// `available`, so a theme can be dropped from the picker without becoming an
// invalid value — still valid, still normalizes to itself, still paintable by
// hand-editing config.json. All three themes ship today and are available;
// the field stays in place as the mechanism for gating a future theme
// (mid-migration, not yet ready for the picker) without touching this shape.
//
// Dependency-free so it runs under `node --test`.

export const THEMES = Object.freeze([
  Object.freeze({ id: 'blue', label: 'Blue', available: true }),
  Object.freeze({ id: 'dark', label: 'Dark', available: true }),
  Object.freeze({ id: 'light', label: 'Light', available: true }),
])

export const AVAILABLE_THEMES = Object.freeze(THEMES.filter((t) => t.available))

export function isValidTheme(v) {
  return typeof v === 'string' && THEMES.some((t) => t.id === v)
}

export function normalizeTheme(v) {
  return isValidTheme(v) ? v : 'blue'
}
