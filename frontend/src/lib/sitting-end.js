// How ReadingView ends its Sitting. Kept out of the component so the rules
// can be tested: the component calls end() from ×, Later, Done, Esc, a switch
// and onDestroy, and those can overlap.
//
// - Ends that overlap share one call. A second close must not re-save a
//   buffer still holding the old # @progress marker.
// - Once ended, the Sitting is not ended again; notes edited after the end
//   are saved on their own.
// - Notes the backend did not save, because the Sitting had already been
//   ended elsewhere, are saved on their own too.
// - A failed end leaves the Sitting open, and the next end tries again.
// - With no Collection open there is nothing left to end. That counts as
//   ended unless there are notes to lose, which the user must decide on.

const NO_COLLECTION = /no collection open/

// endSitting(action, notes) and saveNotes(notes) resolve to the final notes
// content ("" when nothing was saved). end(action, notes) resolves to
// { ok: true, final } or { ok: false, error }.
export function createSittingEnd({ endSitting, saveNotes }) {
  let inFlight = null
  let ended = false

  async function run(action, notes) {
    try {
      let final = ''
      if (!ended) final = await endSitting(action, notes)
      ended = true
      if (notes != null && !final) final = await saveNotes(notes)
      return { ok: true, final: final ?? '' }
    } catch (e) {
      const error = String(e?.message ?? e)
      if (notes == null && NO_COLLECTION.test(error)) {
        ended = true
        return { ok: true, final: '' }
      }
      return { ok: false, error }
    }
  }

  return function end(action, notes) {
    if (!inFlight) inFlight = run(action, notes).finally(() => { inFlight = null })
    return inFlight
  }
}
