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
 * Every double-quoted media path on one line, with offsets relative to it.
 * @returns {{path: string, kind: string, from: number, to: number}[]}
 */
export function findMedia(lineText) {
  const out = []
  // [^"\n]* rather than .*? so an unterminated quote simply fails to match
  // instead of swallowing the rest of the line.
  const re = /"([^"\n]*)"/g
  let m
  while ((m = re.exec(String(lineText ?? ''))) !== null) {
    const kind = mediaKind(m[1])
    if (!kind) continue
    out.push({ path: m[1], kind, from: m.index + 1, to: m.index + 1 + m[1].length })
  }
  return out
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
