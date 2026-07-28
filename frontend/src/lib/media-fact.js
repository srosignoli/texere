// Builds the N4L text promoted from a media cell. Kept as a pure function so it
// is unit-testable without mounting Svelte.
//
// The asset path is collection-relative (`media/<hash>.<ext>`), not absolute and
// not a file:// URL, so a collection stays portable when moved or shared.

// N4L references arrows by their short CODE, not their long name. These are the
// tokens declared in SSTconfig/arrows-EP-3.sst:
//   pic  = "has picture"        pron = "has pronunciation"      aud = "has audio"
const PICTURE_ARROW = 'pic'
const PRONUNCIATION_ARROW = 'pron'
const AUDIO_ARROW = 'aud'

// mediaArrow picks the arrow for a cell. Provenance chooses only a DEFAULT in
// editable text — the promoted draft is an N4L cell the user is about to read —
// so a wrong guess costs one keystroke rather than hiding anything.
// `kind` is absent on cells saved before audio existed, and means image.
export function mediaArrow({ kind, provider } = {}) {
  if (kind !== 'audio') return PICTURE_ARROW
  return provider === 'pronunciation' ? PRONUNCIATION_ARROW : AUDIO_ARROW
}

export function buildMediaFact({ subject, asset, kind, provider }) {
  const s = (subject ?? '').trim()
  if (!s || !asset) return ''
  return `${s}\n"  (${mediaArrow({ kind, provider })}) media/${asset}\n`
}
