// Finding media assets referenced from an N4L line.
//
// Detection is by the target's file EXTENSION, never by the arrow's name.
// Arrow names are free text the author writes, so matching them means
// "has an image" works while "illustrated by", "screenshot" or a typo silently
// does not. The extension is a property of the thing being pointed at.
//
// Dependency-free so it runs under `node --test`: this repo has no CodeMirror
// or Svelte component harness, so logic left inside the plugin has no tests.

const KIND = {
  image: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'bmp'],
  video: ['mp4', 'webm', 'mov', 'm4v'],
  audio: ['mp3', 'wav', 'ogg', 'm4a', 'flac'],
}

/** 'image' | 'video' | 'audio' | null */
export function mediaKind(path) {
  const ext = extensionOf(path)
  if (!ext) return null
  for (const kind of Object.keys(KIND)) {
    if (KIND[kind].includes(ext)) return kind
  }
  return null
}

/**
 * Every media target on one N4L line.
 *
 * Anchored on the ARROW, not on quotes. An N4L line is `subject (code) target`,
 * and a target only needs quoting when it contains spaces — in a real
 * collection roughly 140 of 5,450 targets are quoted. Scanning for quoted spans
 * therefore matched the rare form and missed the common one, including every
 * ditto line, where a leading `"` means "same subject as above" rather than
 * opening a quote.
 *
 * The arrow marks WHERE a target is; it never decides whether one counts. That
 * is still the extension's job.
 *
 * @returns {{path: string, kind: string}[]}
 */
export function findMedia(lineText) {
  const line = String(lineText ?? '')
  const out = []
  // Each (code), and the text up to the next one, which is that arrow's target.
  const re = /\(([^()\n]*)\)/g
  let m
  while ((m = re.exec(line)) !== null) {
    const rest = line.slice(re.lastIndex)
    const next = rest.indexOf('(')
    const target = trimQuotes(next < 0 ? rest : rest.slice(0, next))
    const kind = mediaKind(target)
    if (kind) out.push({ path: target, kind })
  }
  return out
}

// Surrounding quotes are stripped independently rather than as a pair, so a
// target left half-quoted by a typo still resolves instead of carrying a stray
// quote into the URL.
function trimQuotes(s) {
  return s.trim().replace(/^"+/, '').replace(/"+$/, '').trim()
}

/**
 * The `src` for a referenced asset.
 *
 * A remote URL is used as-is. Everything else is served by the Go asset server
 * from the collection directory — which is why nothing here decodes bytes any
 * more: http.ServeFile gives native range requests, so a video seeks with no
 * work at all, and the browser does the caching.
 */
export function srcFor(path) {
  const s = String(path ?? '')
  if (/^https?:\/\//i.test(s)) return s
  // Per segment, not whole-string: encoding the slashes too would mean the
  // server never finds the file, while leaving a '#' or '?' raw would truncate
  // the request at a fragment or query that is really part of the filename.
  return '/collection/' + s.split('/').map(encodeURIComponent).join('/')
}

// The final dot's suffix, lowercased. A name with no dot, or a trailing dot,
// has no extension — "notes-about-png.txt" must not read as an image.
function extensionOf(path) {
  const s = String(path ?? '')
  const dot = s.lastIndexOf('.')
  if (dot < 0 || dot === s.length - 1) return ''
  return s.slice(dot + 1).toLowerCase()
}
