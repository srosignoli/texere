import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createSittingEnd } from './sitting-end.js'

// A deferred promise, so a test decides when the backend answers.
function deferred() {
  let resolve, reject
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}

function fakes() {
  const calls = { end: [], save: [] }
  let next = null
  return {
    calls,
    answer(d) { next = d },
    endSitting: (action, notes) => { calls.end.push({ action, notes }); return next ? next.promise : Promise.resolve(notes == null ? '' : notes + '#marker') },
    saveNotes: (notes) => { calls.save.push(notes); return Promise.resolve(notes) },
  }
}

test('a second end while the first is in flight shares it: no second End, no notes save', async () => {
  const f = fakes()
  const d = deferred()
  f.answer(d)
  const end = createSittingEnd(f)

  const first = end('continue', 'edited')
  const second = end('continue', 'edited')
  d.resolve('edited#marker')
  const [a, b] = await Promise.all([first, second])

  assert.equal(f.calls.end.length, 1)
  assert.equal(f.calls.save.length, 0, 'the second close must not re-save a buffer holding the old marker')
  assert.deepEqual(a, { ok: true, final: 'edited#marker' })
  assert.deepEqual(b, a)
})

test('notes edited after the Sitting ended are saved on their own', async () => {
  const f = fakes()
  const end = createSittingEnd(f)
  await end('continue', null)

  const r = await end('continue', 'typed later')

  assert.equal(f.calls.end.length, 1, 'an ended Sitting is not ended again')
  assert.deepEqual(f.calls.save, ['typed later'])
  assert.deepEqual(r, { ok: true, final: 'typed later' })
})

test('notes the backend did not save (the Sitting was ended elsewhere) are saved on their own', async () => {
  const f = fakes()
  const d = deferred()
  f.answer(d)
  const end = createSittingEnd(f)

  const p = end('continue', 'mine')
  d.resolve('')
  const r = await p

  assert.deepEqual(f.calls.save, ['mine'])
  assert.deepEqual(r, { ok: true, final: 'mine' })
})

test('a failed end leaves the Sitting open, and the next end tries again', async () => {
  const f = fakes()
  const d = deferred()
  f.answer(d)
  const end = createSittingEnd(f)

  const p = end('continue', 'x')
  d.reject(new Error('sitting: save notes: read-only'))
  const r = await p
  assert.equal(r.ok, false)
  assert.match(r.error, /read-only/)

  f.answer(null)
  const again = await end('continue', null)
  assert.equal(again.ok, true)
  assert.equal(f.calls.end.length, 2)
})

test('with no Collection open, an end with nothing to save counts as ended', async () => {
  const f = fakes()
  const d = deferred()
  f.answer(d)
  const end = createSittingEnd(f)

  const p = end('continue', null)
  d.reject('no collection open')
  assert.deepEqual(await p, { ok: true, final: '' })
})

test('with no Collection open, unsaved notes are a failure the user must decide on', async () => {
  const f = fakes()
  const d = deferred()
  f.answer(d)
  const end = createSittingEnd(f)

  const p = end('continue', 'notes')
  d.reject('no collection open')
  assert.equal((await p).ok, false)
})
