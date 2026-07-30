import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildShellFact, sanitizeSubject, sanitizeTarget } from './shell-fact.js'

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

// `//` opens an inline comment on every line type, so the parser drops
// everything after it. URLs are exempt because the parser exempts a slash run
// that follows a colon.
// Brackets do not help here — `[//]` truncates just like `//` — so the slashes
// are separated instead. URLs are exempt because the parser exempts a slash run
// that follows a colon.
test('a bare // is separated but a URL is left alone', () => {
  assert.equal(sanitizeTarget('// TODO: fix this'), '/ / TODO: fix this')
  assert.equal(sanitizeTarget('a // b'), 'a / / b')
  assert.equal(sanitizeTarget('a///b'), 'a/ / /b')
  assert.equal(sanitizeTarget('see http://example.com/x'), 'see http://example.com/x')
  assert.equal(sanitizeTarget('file:///etc/hosts'), 'file:///etc/hosts')
  assert.equal(sanitizeTarget('/var/log/nginx/access.log'), '/var/log/nginx/access.log')
  assert.equal(sanitizeSubject('check //prod// hosts'), 'check / /prod/ / hosts')
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

// A subject IS first on its line, so every leading marker is live — the exact
// opposite of a target. Proven against the real Go parser in
// internal/content/shell_target_test.go.
test('a leading N4L marker in the subject is neutralised, not passed through', () => {
  assert.equal(sanitizeSubject('# incident 4312'), '[#] incident 4312')
  assert.equal(sanitizeSubject('- prod runbook'), '[-] prod runbook')
  assert.equal(sanitizeSubject(':: prod ::'), '[::] prod ::')
  assert.equal(sanitizeSubject('+:: staging'), '[+::] staging')
  assert.equal(sanitizeSubject('@prod notes'), '[@]prod notes')
  assert.equal(sanitizeSubject('"quoted title'), '["]quoted title')
  assert.equal(sanitizeSubject("'quoted title"), "[']quoted title")
  assert.equal(sanitizeSubject('#@ marker line'), '[#@] marker line')
})

// " #" opens a trailing comment on a node line, so it truncates a subject
// wherever it appears — not only at the front. Verified against the real parser.
test('a mid-line " #" is neutralised so the subject is not truncated', () => {
  assert.equal(sanitizeSubject('incident #4312 - prod'), 'incident [#]4312 - prod')
  assert.equal(sanitizeSubject('a # b # c'), 'a [#] b [#] c')
})

// Markers that are neither leading nor preceded by a space are ordinary text.
test('markers away from the front of the subject are left alone', () => {
  assert.equal(sanitizeSubject('incident#4312'), 'incident#4312')
  assert.equal(sanitizeSubject('prod - runbook'), 'prod - runbook')
  assert.equal(sanitizeSubject('mail@example.com check'), 'mail@example.com check')
})

// (...) is arrow syntax wherever it appears, subject included.
test('parentheses in the subject become brackets', () => {
  assert.equal(sanitizeSubject('restart (prod) db'), 'restart [prod] db')
  assert.equal(sanitizeSubject('(prod) restart'), '[prod] restart')
})

test('the subject is trimmed and normalised like a target', () => {
  assert.equal(sanitizeSubject('  padded  '), 'padded')
  assert.equal(sanitizeSubject('a\tb'), 'a b')
  assert.equal(sanitizeSubject('x'.repeat(900)).length, 400)
})

test('an empty or unusable subject still means no fact', () => {
  assert.equal(sanitizeSubject(''), '')
  assert.equal(sanitizeSubject('   '), '')
  assert.equal(sanitizeSubject(null), '')
  assert.equal(buildShellFact({ subject: '\t\r', command: 'c', lines: ['a'] }), '')
})

test('buildShellFact writes the sanitised subject, not the raw one', () => {
  assert.equal(
    buildShellFact({ subject: '# incident 4312', command: 'df -h', lines: ['/dev/sda1'] }),
    '[#] incident 4312\n"  (cmd) df -h\n"  (out) /dev/sda1\n')
})
