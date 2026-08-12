import { test } from 'node:test'
import assert from 'node:assert/strict'
import { mediaKind, srcFor, mediaTokens, stripMediaTokens } from './n4l-media.js'

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

test('mediaTokens finds a path that is the whole string', () => {
  assert.deepEqual(mediaTokens('media/apple.jpg'), ['media/apple.jpg'])
})

test('mediaTokens finds a path surrounded by prose', () => {
  assert.deepEqual(mediaTokens('the fruit media/apple.jpg in profile'), ['media/apple.jpg'])
})

test('mediaTokens finds every path in the string, in order', () => {
  assert.deepEqual(
    mediaTokens('media/a.png then clips/b.mp4 then sound/c.mp3'),
    ['media/a.png', 'clips/b.mp4', 'sound/c.mp3'],
  )
})

test('mediaTokens ignores tokens that are not media', () => {
  assert.deepEqual(mediaTokens('see notes.txt and archive.zip'), [])
})

test('mediaTokens returns empty for prose, empty and nullish input', () => {
  assert.deepEqual(mediaTokens('just some prose'), [])
  assert.deepEqual(mediaTokens(''), [])
  assert.deepEqual(mediaTokens(null), [])
  assert.deepEqual(mediaTokens(undefined), [])
})

test('stripMediaTokens removes a path from the middle without doubling the space', () => {
  assert.equal(stripMediaTokens('the fruit media/apple.jpg in profile'), 'the fruit in profile')
})

test('stripMediaTokens leaves no leading or trailing space', () => {
  assert.equal(stripMediaTokens('media/apple.jpg is a fruit'), 'is a fruit')
  assert.equal(stripMediaTokens('a fruit media/apple.jpg'), 'a fruit')
})

test('stripMediaTokens empties a string that is only a path', () => {
  assert.equal(stripMediaTokens('media/apple.jpg'), '')
  assert.equal(stripMediaTokens('  clips/b.mp4  '), '')
})

test('stripMediaTokens removes every path', () => {
  assert.equal(stripMediaTokens('x media/a.png y clips/b.mp4 z'), 'x y z')
})

// The common card has no media at all. Reformatting its whitespace — collapsing
// the spacing inside a formula, say — would be a visible change to text this
// function has no business touching.
test('stripMediaTokens returns text with no media byte-for-byte unchanged', () => {
  assert.equal(stripMediaTokens('a  double  spaced  line '), 'a  double  spaced  line ')
  assert.equal(stripMediaTokens('\\( E = mc^2 \\)'), '\\( E = mc^2 \\)')
})

test('stripMediaTokens handles empty and nullish input', () => {
  assert.equal(stripMediaTokens(''), '')
  assert.equal(stripMediaTokens(null), '')
  assert.equal(stripMediaTokens(undefined), '')
})

// stripMediaTokens removes EVERY occurrence of a repeated path. If mediaTokens
// were ever deduped (e.g. with a Set), a card naming the same image twice would
// still have both occurrences stripped from the text but only one <InlineMedia>
// rendered — the second occurrence would just vanish instead of showing a
// picture. This pins the duplicate-keeping contract mediaTokens promises.
test('mediaTokens keeps duplicate occurrences of the same path', () => {
  assert.deepEqual(mediaTokens('a/x.png and a/x.png'), ['a/x.png', 'a/x.png'])
})

// The feature's real invariant: whatever mediaTokens reports as media is
// exactly what stripMediaTokens has already removed from its output. If the
// two ever drifted — say stripMediaTokens stopped after the first match while
// mediaTokens kept finding the rest — a leftover path would sit in the
// "stripped" text right next to the InlineMedia already rendered for it,
// showing the path twice over (once as text, once as a picture).
test('stripMediaTokens leaves nothing that mediaTokens still reports as media', () => {
  const samples = [
    'the fruit media/apple.jpg in profile',
    'media/a.png then clips/b.mp4 then sound/c.mp3',
    'a/x.png and a/x.png',
    'just some prose',
    'media/apple.jpg',
  ]
  for (const s of samples) {
    const tokens = new Set(mediaTokens(s))
    const remaining = stripMediaTokens(s).split(/\s+/)
    for (const word of remaining) {
      assert.ok(!tokens.has(word), `${JSON.stringify(word)} left in stripMediaTokens(${JSON.stringify(s)}) but mediaTokens still reports it`)
    }
  }
})
