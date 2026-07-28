// Turns an HTMLMediaElement's MediaError into a message that names the real
// cause. Shared by the media cell's search preview and its audio player, so the
// wording cannot drift between them.
//
// Codes 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean this webview could not play
// the resource at all, which is a completely different remedy from a file that
// failed to load (code 2).
//
// The wording deliberately does NOT say "codec". Code 4 also fires when the
// media is perfectly encoded but unreadable — notably when the server sends no
// Accept-Ranges and the player cannot seek to the MP4 moov atom. A real H.264 /
// AAC file was misdiagnosed as a codec problem for exactly that reason, so the
// message now names both possibilities instead of guessing at one.
export function mediaErrorMessage(err, what = 'Audio') {
  const code = err?.code ?? 0
  if (code === 3 || code === 4) {
    return `${what}: this webview could not play it (MediaError ${code}) — the format may be unsupported, or the server may not allow range requests.`
  }
  if (code === 2) {
    return `${what} failed to load (MediaError ${code}).`
  }
  return `${what} error (MediaError ${code}).`
}
