// Builds the N4L text promoted from a media cell. Kept as a pure function so it
// is unit-testable without mounting Svelte.
//
// The asset path is collection-relative (`media/<hash>.<ext>`), not absolute and
// not a file:// URL, so a collection stays portable when moved or shared.

// N4L arrow declared in SSTconfig/arrows-EP-3.sst. Undeclared arrows still work
// (internal/content/unknown_arrows.go synthesizes them), but declaring it gives
// the relation a proper type and reverse name.
const PICTURE_ARROW = 'has picture'

export function buildMediaFact({ subject, asset }) {
  const s = (subject ?? '').trim()
  if (!s || !asset) return ''
  return `${s}\n    ${PICTURE_ARROW}   media/${asset}\n`
}
