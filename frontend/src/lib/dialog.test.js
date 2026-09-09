import { test } from 'node:test'
import assert from 'node:assert/strict'
import { get } from 'svelte/store'

import { dialogRequest, askText, askConfirm, showAlert, settle, cancelValue } from './dialog.js'

function reset() {
  const open = get(dialogRequest)
  if (open) settle(open, cancelValue(open.kind))
}

test('askText publishes a request and resolves with the typed value', async () => {
  reset()
  const answered = askText('New file name', 'notes.n4l')
  const req = get(dialogRequest)
  assert.equal(req.kind, 'text')
  assert.equal(req.title, 'New file name')
  assert.equal(req.value, 'notes.n4l', 'the suggested value reaches the field')
  settle(req, 'other.n4l')
  assert.equal(await answered, 'other.n4l')
})

// The whole point of the contract: a cancelled prompt must not read as an
// empty answer, or "+file" would create a file named "".
test('a cancelled text prompt resolves null, not empty string', async () => {
  reset()
  const answered = askText('New file name')
  const req = get(dialogRequest)
  settle(req, cancelValue(req.kind))
  assert.equal(await answered, null)
})

test('confirm resolves true only when accepted', async () => {
  reset()
  const yes = askConfirm('Discard?')
  settle(get(dialogRequest), true)
  assert.equal(await yes, true)

  const no = askConfirm('Discard?')
  const req = get(dialogRequest)
  settle(req, cancelValue(req.kind))
  assert.equal(await no, false, 'cancelling a confirm must never mean yes')
})

test('an alert resolves when acknowledged and offers no false answer', async () => {
  reset()
  const done = showAlert('Could not create file')
  const req = get(dialogRequest)
  assert.equal(req.kind, 'alert')
  settle(req, undefined)
  assert.equal(await done, undefined)
})

test('the store is cleared before the promise settles', async () => {
  reset()
  const answered = askConfirm('Close?')
  // A caller opening a second dialog from its own .then() must find the slot
  // free; if settle cleared afterwards it would wipe the new request.
  const chained = answered.then(() => {
    askText('Next question')
    return get(dialogRequest)?.title
  })
  settle(get(dialogRequest), true)
  assert.equal(await chained, 'Next question')
  reset()
})

test('settle ignores a stale request without throwing', () => {
  reset()
  assert.doesNotThrow(() => settle(null, 'anything'))
})

test('cancelValue matches each native dialog it replaces', () => {
  assert.equal(cancelValue('text'), null)        // prompt()
  assert.equal(cancelValue('confirm'), false)    // confirm()
  assert.equal(cancelValue('alert'), undefined)  // alert()
})
