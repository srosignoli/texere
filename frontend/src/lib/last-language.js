// Remembers the last pronunciation language a learner picked, so a new media
// cell can default to it instead of always falling back to the first entry
// in the provider's language list. A learner studying one language stays in
// it for weeks; re-picking it on every new cell is friction with no benefit.
//
// Guarded the same way as ../stores/uiLayout.js: localStorage may not exist
// (SSR, some webview contexts) or may throw (quota, disabled storage), and
// neither case should ever surface to the caller — losing the remembered
// language is not worth breaking the panel over.

const KEY = 'texere.lastLanguage.v1'

export function getLastLanguage() {
  if (typeof localStorage === 'undefined') return ''
  try {
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

export function setLastLanguage(name) {
  if (typeof localStorage === 'undefined') return
  try {
    if (name) localStorage.setItem(KEY, name)
  } catch {
    // ignore quota / storage-disabled failures
  }
}
