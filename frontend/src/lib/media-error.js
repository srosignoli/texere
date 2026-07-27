// Turns an HTMLMediaElement's MediaError into a message that names the real
// cause. Shared by the media cell's search preview and its audio player, so the
// wording cannot drift between them.
//
// Codes 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean this webview cannot play the
// format — common for .ogg under WebKitGTK/WSLg — which is a completely
// different remedy from a file that failed to load.
export function mediaErrorMessage(err, what = 'Audio') {
  const code = err?.code ?? 0
  if (code === 3 || code === 4) {
    return `${what}: format/codec not supported in this webview (MediaError ${code}).`
  }
  if (code === 2) {
    return `${what} failed to load (MediaError ${code}).`
  }
  return `${what} error (MediaError ${code}).`
}
