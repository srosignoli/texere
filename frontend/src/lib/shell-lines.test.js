import { test } from 'node:test'
import assert from 'node:assert/strict'
import { appendChunks, NOTICE_STREAM } from './shell-lines.js'

// Capping now prepends a truncation marker, which is not output. Most tests
// below care about the output lines, so they read through this.
const out = (buf) => buf.filter((l) => l.stream !== NOTICE_STREAM)
const notice = (buf) => buf.find((l) => l.stream === NOTICE_STREAM) || null

test('chunks are split into lines and appended in order', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
  assert.deepEqual(got.map((l) => l.stream), ['stdout', 'stdout'])
})

// Output arrives mid-line: a chunk boundary is not a line boundary.
test('a partial line is completed by the next chunk', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'hel' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hel'])
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'lo\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['hello'])
})

// A partial stdout line must not swallow a stderr chunk that arrives next.
test('a partial line is only continued by the same stream', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'out-partial' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['out-partial', 'err'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stderr'])
})

test('stderr lines keep their stream tag so the UI can tint them', () => {
  const got = appendChunks([], [{ stream: 'stderr', text: 'boom\n' }], 100)
  assert.equal(got[0].stream, 'stderr')
})

test('carriage returns are stripped so CRLF output does not gain blank lines', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\r\nb\r\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', 'b'])
})

// Without this, `yes` exhausts the webview.
test('the buffer is capped, dropping the oldest lines', () => {
  let buf = []
  for (let i = 0; i < 20; i++) buf = appendChunks(buf, [{ stream: 'stdout', text: `${i}\n` }], 5)
  assert.equal(out(buf).length, 5)
  assert.deepEqual(out(buf).map((l) => l.text), ['15', '16', '17', '18', '19'])
})

test('a single oversized chunk is also capped', () => {
  const text = Array.from({ length: 500 }, (_, i) => `l${i}`).join('\n') + '\n'
  const got = appendChunks([], [{ stream: 'stdout', text }], 10)
  assert.equal(out(got).length, 10)
  assert.equal(out(got)[9].text, 'l499')
})

// The design says the buffer drops from the front "with a truncation marker".
// Without one a reader cannot tell a short command from a truncated one.
test('dropping from the front leaves a truncation marker', () => {
  const text = Array.from({ length: 500 }, (_, i) => `l${i}`).join('\n') + '\n'
  const got = appendChunks([], [{ stream: 'stdout', text }], 10)
  const n = notice(got)
  assert.ok(n, 'expected a truncation marker')
  assert.equal(got[0], n, 'the marker belongs at the front, where the loss happened')
  assert.equal(n.dropped, 490)
  assert.match(n.text, /490 earlier lines dropped/)
})

test('no truncation means no marker', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\nb\n' }], 10)
  assert.equal(notice(got), null)
})

// The marker must not itself consume a slot, or "the last N lines" stops being
// true, and it must not re-trigger truncation on every subsequent append.
test('the marker is extra to the cap and does not compound', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'a\nb\nc\nd\n' }], 2)
  assert.deepEqual(out(buf).map((l) => l.text), ['c', 'd'])
  assert.equal(notice(buf).dropped, 2)
  assert.equal(buf.length, 3)

  buf = appendChunks(buf, [{ stream: 'stdout', text: 'e\n' }], 2)
  assert.deepEqual(out(buf).map((l) => l.text), ['d', 'e'])
  assert.equal(buf.length, 3, 'still one marker, not two')
  assert.equal(buf.filter((l) => l.stream === NOTICE_STREAM).length, 1)
})

// Successive truncations accumulate rather than each reporting only its batch.
test('the marker counts every line ever dropped', () => {
  let buf = []
  for (let i = 0; i < 20; i++) buf = appendChunks(buf, [{ stream: 'stdout', text: `${i}\n` }], 3)
  assert.equal(notice(buf).dropped, 17)
  assert.deepEqual(out(buf).map((l) => l.text), ['17', '18', '19'])
})

// Nothing a command prints can forge a marker: appendChunks coerces every
// incoming stream to stdout or stderr.
test('a command cannot forge a truncation marker', () => {
  const got = appendChunks([], [{ stream: NOTICE_STREAM, text: 'fake\n' }], 10)
  assert.deepEqual(got.map((l) => l.stream), ['stdout'])
  assert.equal(notice(got), null)
})

test('empty and malformed input leaves the buffer alone', () => {
  const buf = [{ stream: 'stdout', text: 'keep' }]
  assert.deepEqual(appendChunks(buf, [], 100), buf)
  assert.deepEqual(appendChunks(buf, null, 100), buf)
  assert.deepEqual(appendChunks(buf, [{ stream: 'stdout', text: '' }], 100), buf)
})

test('blank lines within output are preserved', () => {
  const got = appendChunks([], [{ stream: 'stdout', text: 'a\n\nb\n' }], 100)
  assert.deepEqual(got.map((l) => l.text), ['a', '', 'b'])
})

// BUG 2b: Blank line dropped when chunk boundary falls on it
test('blank line is preserved even when split across chunk boundaries', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'a\n' }], 100)
  buf = appendChunks(buf, [{ stream: 'stdout', text: '\nb\n' }], 100)
  // The \n at the start of the second chunk should result in a blank line
  assert.deepEqual(buf.map((l) => l.text), ['a', '', 'b'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stdout', 'stdout'])
})

// BUG 2a: Interleaved partial writes orphan a line
test('interleaved streams do not orphan partial lines', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'AAA' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'BBB' }], 100)
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'CCC\n' }], 100)
  // The AAA and CCC from stdout should be joined, despite BBB from stderr in between
  assert.deepEqual(buf.map((l) => l.text), ['AAACCC', 'BBB'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stderr'])
})

// Single-stream byte-stream invariance (weaker test, but still useful).
test('single-stream result is independent of chunk boundary positions', () => {
  const output = 'line1\nline2\nline3\n'

  // Split 1: every character
  let result1 = []
  for (let i = 0; i < output.length; i++) {
    result1 = appendChunks(result1, [{ stream: 'stdout', text: output[i] }], 100)
  }

  // Split 2: every other chunk
  let result2 = []
  for (let i = 0; i < output.length; i += 2) {
    const chunk = output.slice(i, Math.min(i + 2, output.length))
    result2 = appendChunks(result2, [{ stream: 'stdout', text: chunk }], 100)
  }

  // Split 3: all at once
  const result3 = appendChunks([], [{ stream: 'stdout', text: output }], 100)

  const getText = (buf) => buf.map((l) => l.text)
  assert.deepEqual(getText(result1), getText(result2), 'split-per-char should match split-per-2-chars')
  assert.deepEqual(getText(result2), getText(result3), 'split-per-2-chars should match all-at-once')
})

// Byte-stream invariance with interleaved streams: same total bytes per stream,
// but different chunk boundaries and different interleaving orders, must produce
// identical final buffers. This exercises the per-stream open-line tracking.
test('interleaved byte-stream result is independent of chunk boundaries', () => {
  // Fixed byte sequence per stream (in order):
  // stdout: "out1\nout2\npartial-out"
  // stderr: "err1\nerr2\npartial-err"

  // Arrangement 1: chunks split exactly on newlines, alternating streams
  let result1 = []
  result1 = appendChunks(result1, [{ stream: 'stdout', text: 'out1\n' }], 100)
  result1 = appendChunks(result1, [{ stream: 'stderr', text: 'err1\n' }], 100)
  result1 = appendChunks(result1, [{ stream: 'stdout', text: 'out2\n' }], 100)
  result1 = appendChunks(result1, [{ stream: 'stderr', text: 'err2\n' }], 100)
  result1 = appendChunks(result1, [{ stream: 'stdout', text: 'partial-out' }], 100)
  result1 = appendChunks(result1, [{ stream: 'stderr', text: 'partial-err' }], 100)

  // Arrangement 2: chunks split mid-line, alternating streams (same interleaving)
  let result2 = []
  result2 = appendChunks(result2, [{ stream: 'stdout', text: 'out' }], 100)
  result2 = appendChunks(result2, [{ stream: 'stderr', text: 'err' }], 100)
  result2 = appendChunks(result2, [{ stream: 'stdout', text: '1\nout2' }], 100)
  result2 = appendChunks(result2, [{ stream: 'stderr', text: '1\nerr2' }], 100)
  result2 = appendChunks(result2, [{ stream: 'stdout', text: '\npartial-out' }], 100)
  result2 = appendChunks(result2, [{ stream: 'stderr', text: '\npartial-err' }], 100)

  // Both arrangements should produce identical buffers (same object references)
  assert.deepEqual(result1, result2, 'different chunk boundaries should produce identical results with interleaved streams')
})

// Variant of bug 2a: stderr in the middle of an open stdout line
test('stderr does not close or interfere with open stdout lines', () => {
  let buf = appendChunks([], [{ stream: 'stdout', text: 'out-' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err\n' }], 100)
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'put\n' }], 100)
  assert.deepEqual(buf.map((l) => l.text), ['out-put', 'err'])
  assert.deepEqual(buf.map((l) => l.stream), ['stdout', 'stderr'])
})

// Cap with open lines: an open line should survive truncation if other lines are dropped
test('open line survives when buffer is capped', () => {
  let buf = []
  // Fill past cap with completed lines
  for (let i = 0; i < 5; i++) {
    buf = appendChunks(buf, [{ stream: 'stdout', text: `line${i}\n` }], 3)
  }
  // Should have the 3 most recent completed lines
  assert.deepEqual(out(buf).map((l) => l.text), ['line2', 'line3', 'line4'])
  // Now send an open line (no closing newline)
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'PARTIAL' }], 3)
  // The open line should exist despite the cap
  assert.equal(buf[buf.length - 1].text, 'PARTIAL')
  assert.equal(buf[buf.length - 1].open, true)
  // Close it
  buf = appendChunks(buf, [{ stream: 'stdout', text: '\n' }], 3)
  assert.deepEqual(out(buf).map((l) => l.text), ['line3', 'line4', 'PARTIAL'])
})

// When an open line gets evicted by the cap, continuation must start fresh
test('evicted open line is not resurrected; continuation starts fresh', () => {
  let buf = []
  // Open an stdout line
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'out' }], 2)
  assert.equal(buf[0].open, true)
  // Push enough stderr to evict the stdout line
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err1\nerr2\nerr3\n' }], 2)
  // stdout line is gone; buffer has only the 2 most recent stderr lines
  assert.equal(out(buf).length, 2)
  assert.deepEqual(out(buf).map((l) => l.stream), ['stderr', 'stderr'])
  // Now send continuation for stdout; it should NOT rejoin the evicted line
  buf = appendChunks(buf, [{ stream: 'stdout', text: '-continuation\n' }], 2)
  // The continuation should start a new line, not resurrect "out"
  const stdoutLines = buf.filter((l) => l.stream === 'stdout')
  assert.equal(stdoutLines.length, 1)
  assert.equal(stdoutLines[0].text, '-continuation')
})

// Both streams open at once, under a cap, completing interleaved
test('both streams open simultaneously under cap with interleaved completion', () => {
  let buf = []
  // Open both streams with partial lines
  buf = appendChunks(buf, [{ stream: 'stdout', text: 'out' }], 100)
  buf = appendChunks(buf, [{ stream: 'stderr', text: 'err' }], 100)
  assert.equal(buf[0].open, true)
  assert.equal(buf[1].open, true)

  // Now push more from both streams, interleaved, all at cap 2
  buf = appendChunks(buf, [{ stream: 'stdout', text: '-1\nout-2' }], 2)
  buf = appendChunks(buf, [{ stream: 'stderr', text: '-1\nerr-2' }], 2)

  // Both open lines should exist (cap allows 2 total, both are open)
  const openLines = buf.filter((l) => l.open)
  assert.equal(openLines.length, 2)
  const outOpen = openLines.find((l) => l.stream === 'stdout')
  const errOpen = openLines.find((l) => l.stream === 'stderr')
  assert.equal(outOpen.text, 'out-2')
  assert.equal(errOpen.text, 'err-2')
})
