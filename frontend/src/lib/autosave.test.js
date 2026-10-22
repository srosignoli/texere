import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createAutosave } from './autosave.js'

// Timers the test fires by hand.
function timers() {
  let pending = null
  return {
    set: (fn) => { pending = fn; return 1 },
    clear: () => { pending = null },
    fire: async () => { const fn = pending; pending = null; await fn?.() },
    get pending() { return pending != null },
  }
}

function setup(save) {
  const t = timers()
  const events = []
  const a = createAutosave({
    save, timers: t,
    onError: (e) => events.push(['error', String(e?.message ?? e)]),
    onSaved: () => events.push(['saved']),
  })
  return { a, t, events }
}

test('a burst of changes saves once, after the delay', async () => {
  let saves = 0
  const s = setup(async () => { saves++ })
  s.a.schedule()
  s.a.schedule()
  s.a.schedule()
  assert.equal(saves, 0, 'nothing is saved before the delay')
  await s.t.fire()
  assert.equal(saves, 1)
})

test('a failed save is reported', async () => {
  const s = setup(async () => { throw new Error('database is read-only') })
  s.a.schedule()
  await s.t.fire()
  assert.deepEqual(s.events, [['error', 'database is read-only']])
})

test('the next successful save reports recovery', async () => {
  let fail = true
  const s = setup(async () => { if (fail) throw new Error('read-only') })
  s.a.schedule()
  await s.t.fire()
  fail = false
  s.a.schedule()
  await s.t.fire()
  assert.deepEqual(s.events, [['error', 'read-only'], ['saved']])
})

// On close the pending edits are saved at once; with nothing pending, closing
// writes nothing.
test('flush saves now when a save is pending, and only then', async () => {
  let saves = 0
  const s = setup(async () => { saves++ })
  await s.a.flush()
  assert.equal(saves, 0, 'nothing was pending')
  s.a.schedule()
  await s.a.flush()
  assert.equal(saves, 1)
  assert.equal(s.t.pending, false, 'the timer is cancelled, so the save does not run twice')
})

test('flush reports its failure to onError too', async () => {
  const s = setup(async () => { throw new Error('disk full') })
  s.a.schedule()
  await s.a.flush()
  assert.deepEqual(s.events, [['error', 'disk full']])
})
