import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaErrorMessage } from './media-error.js'

// MediaError 3 (DECODE) and 4 (SRC_NOT_SUPPORTED) mean the resource could not
// be played here, which is a different remedy from "not found". The message
// must NOT blame the codec alone: code 4 also fires for a perfectly encoded
// file whose server sends no Accept-Ranges, and a real H.264/AAC video was
// misdiagnosed that way.
test('unplayable media names both possible causes, not just the codec', () => {
  for (const code of [3, 4]) {
    const msg = mediaErrorMessage({ code })
    assert.match(msg, /could not play it/)
    assert.match(msg, /format may be unsupported/)
    assert.match(msg, /range requests/)
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
