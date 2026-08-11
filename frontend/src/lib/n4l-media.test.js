import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaKind, srcFor, findMedia } from './n4l-media.js'

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

// The line that exposed the bug, verbatim from a real collection. The leading
// quote is an N4L DITTO marker meaning "same subject as above", not an opening
// quote, and the target is unquoted — which is how ~97% of targets in that
// collection are written.
test('findMedia matches an unquoted target on a ditto line', () => {
  const line = '"  (pic) media/bfbaf6828607d9b94fb54056877497ac32926aa75c09de4abd61bc05ba24e2a7.jpg'
  assert.deepEqual(findMedia(line), [{
    path: 'media/bfbaf6828607d9b94fb54056877497ac32926aa75c09de4abd61bc05ba24e2a7.jpg',
    kind: 'image',
  }])
})

test('findMedia matches a quoted target too', () => {
  assert.deepEqual(findMedia('sunset (has an image) "assets/sunset.jpg"'),
    [{ path: 'assets/sunset.jpg', kind: 'image' }])
})

test('findMedia matches a remote target', () => {
  assert.deepEqual(findMedia('plot (pic) https://example.com/a.png'),
    [{ path: 'https://example.com/a.png', kind: 'image' }])
})

// Detection stays arrow-agnostic: the arrow marks WHERE a target is, never
// whether it counts.
test('findMedia does not care what the arrow is called', () => {
  assert.equal(findMedia('x (illustrated by) a/b.png').length, 1)
  assert.equal(findMedia('x (banana) a/b.png').length, 1)
})

test('findMedia ignores a target with no media extension', () => {
  assert.deepEqual(findMedia('note (means) a bright thing'), [])
  assert.deepEqual(findMedia('paper (source) refs/paper.pdf'), [])
  // A page URL is not a media file.
  assert.deepEqual(findMedia('clip (vid) https://archive.org/details/SVS-3387?start=16'), [])
})

// An extract anchor is a quoted target, but "html char=13643" is no extension.
test('findMedia ignores an extract anchor', () => {
  assert.deepEqual(findMedia('Some passage. (from) "sources/spaced-repetition.html char=13643"'), [])
})

test('findMedia finds several targets on one line, in order', () => {
  assert.deepEqual(findMedia('x (a) one.png (b) two.mp3'),
    [{ path: 'one.png', kind: 'image' }, { path: 'two.mp3', kind: 'audio' }])
})

// Text before the first arrow is the subject, never a target.
test('findMedia ignores a filename in the subject', () => {
  assert.deepEqual(findMedia('see the diagram in overview.png'), [])
})

test('findMedia recovers a path from a half-quoted target', () => {
  assert.deepEqual(findMedia('x (has an image) "assets/sunset.jpg'),
    [{ path: 'assets/sunset.jpg', kind: 'image' }])
})

test('findMedia handles an empty line and a line with no arrow', () => {
  assert.deepEqual(findMedia(''), [])
  assert.deepEqual(findMedia('just a subject line'), [])
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
