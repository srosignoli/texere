import { test } from 'node:test'
import assert from 'node:assert/strict'
import { splitMath } from './n4l-math.js'

test('a string with no math is one text segment', () => {
  assert.deepEqual(splitMath('just some prose'), [{ type: 'text', value: 'just some prose' }])
})

test('inline math is extracted without its delimiters', () => {
  assert.deepEqual(splitMath('\\( x = 1 \\)'), [{ type: 'inline', value: ' x = 1 ' }])
})

test('display math is extracted without its delimiters', () => {
  assert.deepEqual(splitMath('\\[ y = 2 \\]'), [{ type: 'display', value: ' y = 2 ' }])
})

// The real shape in moon.n4l: prose and both kinds of math in one string.
test('prose and math interleave in order', () => {
  const got = splitMath('The momentum \\(L\\) obeys \\[ L = I\\omega \\] where \\(I\\) is inertia.')
  assert.deepEqual(got, [
    { type: 'text', value: 'The momentum ' },
    { type: 'inline', value: 'L' },
    { type: 'text', value: ' obeys ' },
    { type: 'display', value: ' L = I\\omega ' },
    { type: 'text', value: ' where ' },
    { type: 'inline', value: 'I' },
    { type: 'text', value: ' is inertia.' },
  ])
})

test('the real arrow target from moon.n4l', () => {
  assert.deepEqual(splitMath('\\( I = 8.1 \\times 10^{37} kgm^2\\)'),
    [{ type: 'inline', value: ' I = 8.1 \\times 10^{37} kgm^2' }])
})

// Braces inside a formula must not be mistaken for anything structural.
test('braces nested in a formula survive', () => {
  assert.deepEqual(splitMath('\\[ \\frac{mM}{m+M} \\]'),
    [{ type: 'display', value: ' \\frac{mM}{m+M} ' }])
})

// The bug this exists to prevent: an opener with no closer must not consume the
// rest of the string. An unterminated quote did exactly that once here.
test('an unclosed inline delimiter stays prose', () => {
  assert.deepEqual(splitMath('before \\( x = 1 and no closer'),
    [{ type: 'text', value: 'before \\( x = 1 and no closer' }])
})

test('an unclosed display delimiter stays prose', () => {
  assert.deepEqual(splitMath('before \\[ x = 1 and no closer'),
    [{ type: 'text', value: 'before \\[ x = 1 and no closer' }])
})

// $ introduces an N4L variable. Treating it as math would corrupt every
// variable reference in the collection.
test('a dollar variable is left completely alone', () => {
  assert.deepEqual(splitMath('Start (next) $question.1'),
    [{ type: 'text', value: 'Start (next) $question.1' }])
  assert.deepEqual(splitMath('$a.1 and $b.1'), [{ type: 'text', value: '$a.1 and $b.1' }])
})

test('an empty or missing string yields no segments', () => {
  assert.deepEqual(splitMath(''), [])
  assert.deepEqual(splitMath(null), [])
  assert.deepEqual(splitMath(undefined), [])
})

test('two formulas of the same kind in one string', () => {
  assert.deepEqual(splitMath('\\(a\\) then \\(b\\)'), [
    { type: 'inline', value: 'a' },
    { type: 'text', value: ' then ' },
    { type: 'inline', value: 'b' },
  ])
})

// An empty formula is degenerate but must not produce a broken segment.
test('an empty formula yields an empty value, not a missing segment', () => {
  assert.deepEqual(splitMath('\\(\\)'), [{ type: 'inline', value: '' }])
})
