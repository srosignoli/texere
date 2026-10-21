// How StudySurface switches the document in the reading area. Kept out of the
// component so the races can be tested.
//
// - A switch to another document ends the visible one's Sitting first (flush),
//   and every switch waits for an end still in flight, so the next Sitting
//   never begins before the last one is written.
// - The last click wins: a switch overtaken by a newer one shows nothing.
// - A flush that fails (the notes could not be saved) stops the switch, so the
//   reader stays up with its error instead of being torn down.
// - Once a flush has ended the visible Sitting, the view is remounted even if
//   the user clicked back to the same document, so no view outlives its Sitting.

// currentKey() is the visible document's key, flush() resolves to whether the
// visible Sitting ended, show(key, remount, target) displays key.
// switchTo(key, target) resolves to whether it showed key.
export function createSwitcher({ currentKey, flush, show }) {
  let seq = 0
  let pending = null
  let ended = false

  return async function switchTo(key, target) {
    const mine = ++seq
    if (key !== currentKey() && !ended) {
      ended = true
      pending = Promise.resolve(flush()).finally(() => { pending = null })
    }
    if (pending) {
      const ok = await pending
      if (!ok) {
        ended = false
        return false
      }
    }
    if (mine !== seq) return false
    const remount = ended
    ended = false
    show(key, remount, target)
    return true
  }
}
