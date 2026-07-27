import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaErrorMessage } from './media-error.js'

// MediaError 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean the format cannot be
// played here — WebKitGTK/WSLg frequently lacks the codecs for .ogg. Reporting
// that as "not found" would send the user hunting for the wrong problem.
test('codec failures name the format, not a missing file', () => {
  for (const code of [3, 4]) {
    const msg = mediaErrorMessage({ code })
    assert.match(msg, /format\/codec not supported/)
    assert.match(msg, new RegExp(`MediaError ${code}`))
  }
})

test('a network failure is reported as a load failure', () => {
  assert.match(mediaErrorMessage({ code: 2 }), /failed to load/)
})

test('an unknown or absent error still produces a message', () => {
  assert.match(mediaErrorMessage({ code: 0 }), /MediaError 0/)
  assert.match(mediaErrorMessage(null), /MediaError 0/)
  assert.match(mediaErrorMessage(undefined), /MediaError 0/)
})

test('the subject noun is configurable', () => {
  assert.match(mediaErrorMessage({ code: 2 }, 'Preview'), /^Preview/)
})
