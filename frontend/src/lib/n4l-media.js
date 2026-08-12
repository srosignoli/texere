// Deciding whether a referenced path is media, and what URL to fetch it from.
//
// Detection is by the path's file EXTENSION, never by the arrow's name. Arrow
// names are free text the author writes, so matching them would mean
// "has an image" works while "illustrated by", "pic" or a typo silently does
// not. The extension is a property of the thing being pointed at.
//
// Dependency-free so it runs under `node --test`: this repo has no Svelte
// component harness, so logic left inside a component has no tests. The Read
// tab's InlineMedia is deliberately thin glue over these two functions.

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

/**
 * The whitespace-delimited tokens of `text` that are media paths.
 *
 * Lived in NodeCard until the Recall panel needed it too. In a component it had
 * no tests at all — this repo runs `node --test` over this directory only.
 */
export function mediaTokens(text) {
  return String(text ?? '').split(/\s+/).filter((t) => mediaKind(t))
}

/**
 * `text` with its media paths removed, for surfaces that show the media in the
 * path's place rather than beside it.
 *
 * Text containing no media is returned untouched rather than re-joined: almost
 * every card is that case, and silently normalising its whitespace would edit
 * strings this function was not asked to change.
 */
export function stripMediaTokens(text) {
  const s = String(text ?? '')
  const parts = s.split(/\s+/)
  if (!parts.some((t) => mediaKind(t))) return s
  return parts.filter((t) => t && !mediaKind(t)).join(' ')
}
