import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildShellFact, sanitizeTarget } from './shell-fact.js'

test('a fact names the subject, the command, and one line per output line', () => {
  const got = buildShellFact({
    subject: 'check containers on prod',
    command: "docker ps --format '{{.Names}}'",
    lines: ['pg-primary', 'redis-cache'],
  })
  assert.equal(got,
    'check containers on prod\n' +
    '"  (cmd) docker ps --format \'{{.Names}}\'\n' +
    '"  (out) pg-primary\n' +
    '"  (out) redis-cache\n')
})

// (...) is arrow syntax wherever it appears in a target — media-fact.js records
// a parenthesised timestamp being silently eaten by the parser.
test('parentheses are replaced, never passed through', () => {
  assert.equal(sanitizeTarget('at foo(bar) line 3'), 'at foo[bar] line 3')
  assert.equal(sanitizeTarget('(note) not an arrow'), '[note] not an arrow')
  assert.ok(!sanitizeTarget('a(b)c').includes('('))
  assert.ok(!sanitizeTarget('a(b)c').includes(')'))
})

// These are safe only because the ditto prefix means they are never first on
// the line. Asserting it documents why no escaping is needed.
test('leading N4L markers pass through untouched', () => {
  assert.equal(sanitizeTarget('# not a comment here'), '# not a comment here')
  assert.equal(sanitizeTarget('-rw-r--r-- 1 root root'), '-rw-r--r-- 1 root root')
  assert.equal(sanitizeTarget(':: not a section'), ':: not a section')
  assert.equal(sanitizeTarget('@reboot /usr/bin/thing'), '@reboot /usr/bin/thing')
  assert.equal(sanitizeTarget('"quoted"'), '"quoted"')
})

test('tabs and carriage returns become spaces, and edges are trimmed', () => {
  assert.equal(sanitizeTarget('a\tb'), 'a b')
  assert.equal(sanitizeTarget('trailing\r'), 'trailing')
  assert.equal(sanitizeTarget('   padded   '), 'padded')
})

test('an empty or whitespace-only line sanitises to the empty string', () => {
  assert.equal(sanitizeTarget(''), '')
  assert.equal(sanitizeTarget('   '), '')
  assert.equal(sanitizeTarget('\t\r'), '')
  assert.equal(sanitizeTarget(null), '')
  assert.equal(sanitizeTarget(undefined), '')
})

test('an over-long line is capped at 400 characters', () => {
  const long = 'x'.repeat(900)
  assert.equal(sanitizeTarget(long), 'x'.repeat(400))
  assert.equal(sanitizeTarget('y'.repeat(400)), 'y'.repeat(400))
})

test('blank output lines are dropped rather than becoming empty facts', () => {
  const got = buildShellFact({ subject: 's', command: 'c', lines: ['a', '', '   ', 'b'] })
  assert.equal(got, 's\n"  (cmd) c\n"  (out) a\n"  (out) b\n')
})

test('at most 200 output lines are promoted', () => {
  const lines = Array.from({ length: 500 }, (_, i) => `line${i}`)
  const got = buildShellFact({ subject: 's', command: 'c', lines })
  assert.equal(got.split('(out)').length - 1, 200)
  assert.ok(got.includes('"  (out) line199\n'))
  assert.ok(!got.includes('"  (out) line200\n'))
})

// Mirrors buildMediaFact, which returns '' without a subject.
test('no subject means no fact', () => {
  assert.equal(buildShellFact({ subject: '', command: 'c', lines: ['a'] }), '')
  assert.equal(buildShellFact({ subject: '   ', command: 'c', lines: ['a'] }), '')
  assert.equal(buildShellFact({ command: 'c', lines: ['a'] }), '')
})

test('nothing to say means no fact', () => {
  assert.equal(buildShellFact({ subject: 's', command: '', lines: [] }), '')
  assert.equal(buildShellFact({ subject: 's', command: '   ', lines: ['  '] }), '')
  assert.equal(buildShellFact({ subject: 's' }), '')
  assert.equal(buildShellFact(), '')
  assert.equal(buildShellFact(null), '')
})

test('output alone, with no command, still promotes', () => {
  assert.equal(buildShellFact({ subject: 's', command: '', lines: ['a'] }), 's\n"  (out) a\n')
})
