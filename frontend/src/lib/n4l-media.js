// Finding media assets referenced from an N4L line.
//
// Detection is by the target's file EXTENSION, never by the arrow's name.
// Arrow names are free text the author writes, so matching them means
// "has an image" works while "illustrated by", "screenshot" or a typo silently
// does not. The extension is a property of the thing being pointed at.
//
// Dependency-free so it runs under `node --test`: this repo has no CodeMirror
// or Svelte component harness, so logic left inside the plugin has no tests.

const MIME = {
  png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', gif: 'image/gif',
  webp: 'image/webp', svg: 'image/svg+xml', bmp: 'image/bmp',
  mp4: 'video/mp4', webm: 'video/webm', mov: 'video/quicktime', m4v: 'video/x-m4v',
  mp3: 'audio/mpeg', wav: 'audio/wav', ogg: 'audio/ogg', m4a: 'audio/mp4', flac: 'audio/flac',
}

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
 * The Blob's MIME type. A <video> handed a blob with no type will not play, so
 * this is load-bearing rather than cosmetic. '' for anything unrecognised.
 */
export function mediaMime(path) {
  return MIME[extensionOf(path)] ?? ''
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

// The final dot's suffix, lowercased. A name with no dot, or a trailing dot,
// has no extension — "notes-about-png.txt" must not read as an image.
function extensionOf(path) {
  const s = String(path ?? '')
  const dot = s.lastIndexOf('.')
  if (dot < 0 || dot === s.length - 1) return ''
  return s.slice(dot + 1).toLowerCase()
}
