// Builds the N4L text promoted from a media cell. Kept as pure functions so they
// are unit-testable without mounting Svelte.
//
// Stored kinds (image, audio) reference a collection-relative asset path
// (`media/<hash>.<ext>`), so a collection stays portable. Video is referenced
// rather than stored, so it points at the provider's watch page — a durable,
// shareable URL, unlike the CDN file URLs used for playback, which rotate.

// N4L references arrows by their short CODE, not their long name. These are the
// tokens declared in SSTconfig/arrows-EP-3.sst:
//   pic = "has picture"   pron = "has pronunciation"
//   aud = "has audio"     vid  = "has video"
const PICTURE_ARROW = 'pic'
const PRONUNCIATION_ARROW = 'pron'
const AUDIO_ARROW = 'aud'
const VIDEO_ARROW = 'vid'

// mediaArrow picks the arrow for a cell. Provenance chooses only a DEFAULT in
// editable text — the promoted draft is an N4L cell the user is about to read —
// so a wrong guess costs one keystroke rather than hiding anything.
// `kind` is absent on cells saved before audio existed, and means image.
export function mediaArrow({ kind, provider } = {}) {
  if (kind === 'video') return VIDEO_ARROW
  if (kind !== 'audio') return PICTURE_ARROW
  return provider === 'pronunciation' ? PRONUNCIATION_ARROW : AUDIO_ARROW
}

// hasContent reports whether a media cell holds anything yet. Stored kinds set
// `asset`; video sets only `embedURL`, so testing `asset` alone would leave a
// video cell permanently showing its acquisition panel.
export function hasContent(cell) {
  return Boolean(cell?.asset || cell?.embedURL)
}

// buildVideoURL appends the provider's own time parameter to a watch page.
// Each spells it differently, and all four shapes are known to survive the N4L
// parser with the target preserved exactly.
export function buildVideoURL({ sourcePage, provider, startSec } = {}) {
  const url = (sourcePage ?? '').trim()
  if (!url) return ''
  const t = Math.max(0, Math.round(startSec ?? 0))
  if (!t) return url
  if (provider === 'commons-video') return `${url}#t=${t}`
  const sep = url.includes('?') ? '&' : '?'
  if (provider === 'youtube') return `${url}${sep}t=${t}s`
  return `${url}${sep}start=${t}`
}

// clock renders seconds as m:ss, or h:mm:ss past an hour.
// parseClock turns what a user types into seconds, accepting "m:ss", "h:mm:ss"
// and a bare count of seconds. Returns null for anything unusable, so the caller
// can leave the previous value alone rather than silently resetting to zero.
// Minutes and seconds are not range-checked: "3:99" reads as 3m99s, which is
// what someone typing quickly means, and normalising is friendlier than
// refusing.
export function parseClock(text) {
  const s = String(text ?? '').trim()
  if (!s) return null
  const parts = s.split(':')
  if (parts.length > 3) return null
  if (!parts.every((p) => /^\d+$/.test(p.trim()))) return null
  const n = parts.map((p) => parseInt(p, 10))
  const total = n.length === 3
    ? n[0] * 3600 + n[1] * 60 + n[2]
    : n.length === 2
      ? n[0] * 60 + n[1]
      : n[0]
  return Number.isFinite(total) ? total : null
}

// formatClock is the inverse, and the same rendering used in the promoted note.
export function formatClock(sec) {
  return clock(sec)
}

function clock(sec) {
  const s = Math.max(0, Math.round(sec))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

export function buildMediaFact(cell) {
  const subject = (cell?.subject ?? '').trim()
  if (!subject) return ''

  if (cell.kind === 'video') {
    const url = buildVideoURL(cell)
    if (!url) return ''
    let out = `${subject}\n"  (${VIDEO_ARROW}) ${url}\n`
    // A bare URL is unreadable months later, so a title — when there is one —
    // goes on a note line. An empty note line would be worse than none.
    const title = (cell.title ?? '').trim()
    if (title) {
      const t = Math.max(0, Math.round(cell.startSec ?? 0))
      // The clock is appended with "@", never wrapped in parentheses: N4L reads
      // "(...)" as arrow syntax, so a parenthesised clock is silently consumed
      // by the parser and never reaches the graph.
      //
      // "@" is safe here despite N4L's "@alias" syntax, which only applies when
      // "@" is the FIRST character of a line (parser.go's contentSQStart tests
      // HasPrefix on the trimmed line; "# @id" needs a leading "#"). This line
      // always begins with the ditto prefix, so the "@" is ordinary text.
      // Verified: a note target survives intact with "@ 3:43", with "@3:43",
      // and even when the title itself starts with "@".
      out += `"  (note) ${t ? `${title} @ ${clock(t)}` : title}\n`
    }
    return out
  }

  if (!cell.asset) return ''
  return `${subject}\n"  (${mediaArrow(cell)}) media/${cell.asset}\n`
}
