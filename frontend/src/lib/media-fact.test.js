import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMediaFact, mediaArrow, buildVideoURL, hasContent } from './media-fact.js'

test('builds a has-picture fact from subject and asset', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg', kind: 'image', provider: 'commons' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a cell saved before this slice has no kind and stays a picture', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'a3f9c1.jpg' }),
    'manzana\n"  (pic) media/a3f9c1.jpg\n')
})

test('a pronunciation result uses the pron arrow', () => {
  assert.equal(
    buildMediaFact({ subject: 'manzana', asset: 'b7.wav', kind: 'audio', provider: 'pronunciation' }),
    'manzana\n"  (pron) media/b7.wav\n')
})

test('other audio uses the generic aud arrow', () => {
  for (const provider of ['commons-audio', 'file', 'url', '']) {
    assert.equal(
      buildMediaFact({ subject: 'nightingale', asset: 'c9.ogg', kind: 'audio', provider }),
      'nightingale\n"  (aud) media/c9.ogg\n',
      `provider=${provider}`)
  }
})

test('a multi-word subject is preserved verbatim', () => {
  assert.equal(
    buildMediaFact({ subject: 'red delicious apple', asset: 'b7.png', kind: 'image' }),
    'red delicious apple\n"  (pic) media/b7.png\n')
})

test('subject is trimmed', () => {
  assert.equal(
    buildMediaFact({ subject: '  manzana  ', asset: 'a3.jpg', kind: 'image' }),
    'manzana\n"  (pic) media/a3.jpg\n')
})

test('an empty subject yields an empty string, not a malformed fact', () => {
  assert.equal(buildMediaFact({ subject: '', asset: 'a3.jpg', kind: 'image' }), '')
  assert.equal(buildMediaFact({ subject: '   ', asset: 'a3.jpg', kind: 'image' }), '')
})

test('a missing asset yields an empty string', () => {
  assert.equal(buildMediaFact({ subject: 'manzana', asset: '', kind: 'image' }), '')
})

test('mediaArrow is exported so the UI can name the arrow it will write', () => {
  assert.equal(mediaArrow({ kind: 'audio', provider: 'pronunciation' }), 'pron')
  assert.equal(mediaArrow({ kind: 'audio', provider: 'commons-audio' }), 'aud')
  assert.equal(mediaArrow({ kind: 'image', provider: 'commons' }), 'pic')
  assert.equal(mediaArrow({}), 'pic')
})

test('video uses the vid arrow', () => {
  assert.equal(mediaArrow({ kind: 'video', provider: 'peertube' }), 'vid')
  assert.equal(mediaArrow({ kind: 'video', provider: 'youtube' }), 'vid')
})

test('buildVideoURL appends each provider its own time parameter', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://diler.tube/videos/watch/abc', provider: 'peertube', startSec: 223 }),
    'https://diler.tube/videos/watch/abc?start=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://archive.org/details/x', provider: 'archive', startSec: 223 }),
    'https://archive.org/details/x?start=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://commons.wikimedia.org/wiki/File:x.webm', provider: 'commons-video', startSec: 223 }),
    'https://commons.wikimedia.org/wiki/File:x.webm#t=223')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://www.youtube.com/watch?v=abc', provider: 'youtube', startSec: 223 }),
    'https://www.youtube.com/watch?v=abc&t=223s')
})

test('a URL that already has a query gets & rather than a second ?', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w?a=1', provider: 'peertube', startSec: 5 }),
    'https://x.tube/w?a=1&start=5')
})

test('no time parameter is appended when the start is zero', () => {
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w', provider: 'peertube', startSec: 0 }),
    'https://x.tube/w')
  assert.equal(
    buildVideoURL({ sourcePage: 'https://x.tube/w', provider: 'peertube' }),
    'https://x.tube/w')
})

test('buildVideoURL returns empty for a missing source page', () => {
  assert.equal(buildVideoURL({ sourcePage: '', provider: 'peertube', startSec: 5 }), '')
  assert.equal(buildVideoURL({}), '')
})

test('a video fact is the link plus a readable note line', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'peertube',
      sourcePage: 'https://diler.tube/videos/watch/abc',
      title: 'Dr. Binocs Show — Photosynthesis', startSec: 223,
    }),
    'photosynthesis\n"  (vid) https://diler.tube/videos/watch/abc?start=223\n' +
    '"  (note) Dr. Binocs Show — Photosynthesis (3:43)\n')
})

test('the note line carries no time when the start is zero', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'peertube',
      sourcePage: 'https://diler.tube/videos/watch/abc', title: 'Explainer', startSec: 0,
    }),
    'photosynthesis\n"  (vid) https://diler.tube/videos/watch/abc\n"  (note) Explainer\n')
})

test('an untitled video emits no note line rather than an empty one', () => {
  assert.equal(
    buildMediaFact({
      subject: 'photosynthesis', kind: 'video', provider: 'url',
      sourcePage: 'https://example.org/v.mp4', title: '', startSec: 0,
    }),
    'photosynthesis\n"  (vid) https://example.org/v.mp4\n')
})

test('an hour-long start time is clocked as h:mm:ss', () => {
  assert.equal(
    buildMediaFact({
      subject: 'x', kind: 'video', provider: 'peertube',
      sourcePage: 'https://x.tube/w', title: 'Long lecture', startSec: 3723,
    }),
    'x\n"  (vid) https://x.tube/w?start=3723\n"  (note) Long lecture (1:02:03)\n')
})

test('a video with no source page yields no fact', () => {
  assert.equal(
    buildMediaFact({ subject: 'x', kind: 'video', provider: 'peertube', sourcePage: '' }), '')
})

test('hasContent covers both stored assets and referenced URLs', () => {
  assert.equal(hasContent({ asset: 'a3.jpg' }), true)
  assert.equal(hasContent({ embedURL: 'https://x.tube/w' }), true)
  assert.equal(hasContent({ asset: '', embedURL: '' }), false)
  assert.equal(hasContent({}), false)
  assert.equal(hasContent(null), false)
})
