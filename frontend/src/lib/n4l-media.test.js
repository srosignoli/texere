import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaKind, srcFor } from './n4l-media.js'

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











test('srcFor passes a remote URL through untouched', () => {
  assert.equal(srcFor('https://example.com/a.png'), 'https://example.com/a.png')
  assert.equal(srcFor('http://example.com/a.png'), 'http://example.com/a.png')
})

test('srcFor recognises a remote URL whatever its case', () => {
  assert.equal(srcFor('HTTPS://Example.com/A.png'), 'HTTPS://Example.com/A.png')
})

test('srcFor routes a local path through the collection server', () => {
  assert.equal(srcFor('media/a.png'), '/collection/media/a.png')
  assert.equal(srcFor('deep/nest/c.png'), '/collection/deep/nest/c.png')
})

// Encoding is per SEGMENT: whole-string encoding would escape the slashes and
// the server would never find the file.
test('srcFor keeps path separators while encoding the segments', () => {
  assert.equal(srcFor('my folder/a b.png'), '/collection/my%20folder/a%20b.png')
})

// A '#' or '?' inside a filename would otherwise start a fragment or query and
// silently truncate the request.
test('srcFor encodes characters that would truncate the URL', () => {
  assert.equal(srcFor('notes/a#1.png'), '/collection/notes/a%231.png')
  assert.equal(srcFor('notes/a?b.png'), '/collection/notes/a%3Fb.png')
})

test('srcFor handles an empty path without throwing', () => {
  assert.equal(srcFor(''), '/collection/')
})
