import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildShellFact } from './shell-fact.js'
import { sanitizeSubject } from './n4l-escape.js'

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
