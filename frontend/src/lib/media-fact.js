// Builds the N4L text promoted from a media cell. Kept as a pure function so it
// is unit-testable without mounting Svelte.
//
// The asset path is collection-relative (`media/<hash>.<ext>`), not absolute and
// not a file:// URL, so a collection stays portable when moved or shared.

// N4L references arrows by their short CODE, not their long name — the code
// is the `(pic)` token in SSTconfig/arrows-EP-3.sst, which declares long name
// "has picture" / reverse "is a picture of" for it.
const PICTURE_ARROW = 'pic'

export function buildMediaFact({ subject, asset }) {
  const s = (subject ?? '').trim()
  if (!s || !asset) return ''
  return `${s}\n"  (${PICTURE_ARROW}) media/${asset}\n`
}
