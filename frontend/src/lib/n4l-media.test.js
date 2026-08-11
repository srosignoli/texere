import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaKind, mediaMime, findMedia } from './n4l-media.js'

test('mediaKind recognises every supported extension', () => {
  for (const p of ['a.png', 'a.jpg', 'a.jpeg', 'a.gif', 'a.webp', 'a.svg', 'a.bmp']) {
    assert.equal(mediaKind(p), 'image', p)
  }
  for (const p of ['a.mp4', 'a.webm', 'a.mov', 'a.m4v']) {
    assert.equal(mediaKind(p), 'video', p)
  }
  for (const p of ['a.mp3', 'a.wav', 'a.ogg', 'a.m4a', 'a.flac']) {
    assert.equal(mediaKind(p), 'audio', p)
  }
})

test('mediaKind is case-insensitive', () => {
  assert.equal(mediaKind('PHOTO.PNG'), 'image')
  assert.equal(mediaKind('Clip.MP4'), 'video')
})

test('mediaKind rejects everything else', () => {
  for (const p of ['a.pdf', 'a.txt', 'a.n4l', 'noextension', '', 'trailing.']) {
    assert.equal(mediaKind(p), null, JSON.stringify(p))
  }
})

// A .png in the middle of a name is not the extension. Matching anywhere in the
// string would render "notes-about-png.txt" as an image.
test('mediaKind looks at the extension, not the whole path', () => {
  assert.equal(mediaKind('notes-about-png.txt'), null)
  assert.equal(mediaKind('mp4-encoding-guide.md'), null)
  assert.equal(mediaKind('assets/my.png.backup'), null)
  // The discriminating case: only the LAST dot's suffix is the extension.
  // Reading from the first dot gives 'txt.png' here and misses a real image.
  assert.equal(mediaKind('archive.txt.png'), 'image')
})

test('mediaMime maps each kind to a playable type', () => {
  assert.equal(mediaMime('a.png'), 'image/png')
  assert.equal(mediaMime('a.jpg'), 'image/jpeg')
  assert.equal(mediaMime('a.jpeg'), 'image/jpeg')
  assert.equal(mediaMime('a.svg'), 'image/svg+xml')
  assert.equal(mediaMime('a.mp4'), 'video/mp4')
  assert.equal(mediaMime('a.mp3'), 'audio/mpeg')
  assert.equal(mediaMime('a.m4a'), 'audio/mp4')
  assert.equal(mediaMime('a.txt'), '')
})

test('findMedia returns a quoted media path with its offsets', () => {
  const line = 'sunset (has an image) "assets/sunset.jpg"'
  const got = findMedia(line)
  assert.equal(got.length, 1)
  assert.equal(got[0].path, 'assets/sunset.jpg')
  assert.equal(got[0].kind, 'image')
  // Offsets must span the path itself, not the surrounding quotes.
  assert.equal(line.slice(got[0].from, got[0].to), 'assets/sunset.jpg')
})

test('findMedia ignores a quoted string that is not media', () => {
  assert.deepEqual(findMedia('note (means) "a bright thing"'), [])
  assert.deepEqual(findMedia('paper (source) "refs/paper.pdf"'), [])
})

// The arrow name is deliberately irrelevant — that is the whole detection rule.
test('findMedia does not care what the arrow is called', () => {
  assert.equal(findMedia('x (illustrated by) "a/b.png"').length, 1)
  assert.equal(findMedia('x (banana) "a/b.png"').length, 1)
})

test('findMedia finds several matches on one line, in order', () => {
  const got = findMedia('x (a) "one.png" (b) "two.mp3"')
  assert.deepEqual(got.map((m) => m.path), ['one.png', 'two.mp3'])
  assert.deepEqual(got.map((m) => m.kind), ['image', 'audio'])
})

test('findMedia ignores an unquoted path', () => {
  assert.deepEqual(findMedia('x (has an image) assets/sunset.jpg'), [])
})

// N4L files in this collection have been broken by stray quotes before. An
// unterminated quote must yield nothing rather than swallowing the rest of the
// line or hanging.
test('findMedia survives an unterminated quote', () => {
  assert.deepEqual(findMedia('x (has an image) "assets/sunset.jpg'), [])
  assert.deepEqual(findMedia('"" (a) "b.png"'), [{ path: 'b.png', kind: 'image', from: 8, to: 13 }])
})

test('findMedia handles an empty line', () => {
  assert.deepEqual(findMedia(''), [])
})
