// Escaping user text so it survives the N4L parser.
//
// Shared by every "promote to N4L" path — shell cells and media cells today.
// It lives in its own module because duplicating it is exactly how this went
// wrong once already: the shell cell fixed both hazards below and media-fact.js
// silently kept them, so a media subject or video title carrying N4L syntax was
// still being truncated on its way to disk long after the shell cell was safe.
//
// Every claim in the comments here is proven against the real Go parser in
// internal/content/shell_target_test.go, not reasoned about. Two of them were
// only found BY that test after the prose reasoning had concluded otherwise.
//
// Pure and dependency-free so Node's test runner can load it.

// Long enough for a real `ls -l` row or a stack frame, short enough that one
// runaway line cannot dominate the file.
const MAX_LINE = 400

const DOUBLE_SLASH = /(^|[^:/])(\/{2,})/g

// sanitizeTarget makes one line of shell output safe to use as the target of a
// ditto line.
//
// It is narrower than it looks. Because every target is preceded by the ditto
// prefix (`"  (out) `), it is never the first thing on a line — so N4L's
// leading-position markers (`#` comment, `"` ditto, `-` chapter, `::` section,
// `@` alias) are all ordinary text here and pass through untouched. This is the
// same argument media-fact.js makes for a leading `@`.
//
// Parentheses are the real hazard: `(...)` is arrow syntax WHEREVER it appears,
// and media-fact.js records a parenthesised timestamp being silently consumed
// by the parser and never reaching the graph. Square brackets keep the shape
// readable, which matters for a stack trace or a `docker ps` row — deliberately
// lossy rather than dropping the characters.
//
// Newlines are not handled here: the caller splits output into lines first, and
// one line is one fact.
//
// `//` starts an inline comment on EVERY line type, targets included — the
// parser truncates there and the rest of the line never reaches the graph.
// `git log`, `grep` over source and `docker inspect` all emit `//` routinely,
// so this is not theoretical. The parser exempts a slash run that follows a
// colon so URLs survive (`http://host`, `file:///x`), and DOUBLE_SLASH matches
// the same exemption.
//
// Bracketing does NOT work here, unlike everywhere else in this file: the
// comment strip runs before anything looks at brackets, so `[//]` truncates
// exactly as `//` does (`check [//]prod` reaches the graph as `check [`).
// Verified in internal/content/shell_target_test.go. Separating the slashes
// with a space is what actually survives, and it keeps every character.
export function sanitizeTarget(line) {
  let t = String(line ?? '').replace(/[\r\t\n]/g, ' ').trim()
  if (!t) return ''
  t = t.replace(/\(/g, '[').replace(/\)/g, ']')
  t = t.replace(DOUBLE_SLASH, (_, before, slashes) => before + slashes.split('').join(' '))
  return t.length > MAX_LINE ? t.slice(0, MAX_LINE) : t
}

// N4L's leading-position markers, as the parser tests them (internal/content/
// parser.go, processLine):
//   -    chapter title            #    comment, and "#@" / "# @" annotation
//   ::   section, incl. +:: -::   "    ditto
//   @    alias / label prefix     '    opening of a single-quoted block
const SUBJECT_MARKERS = /^[#\-+:@"']+/

// sanitizeSubject makes a user-typed subject safe as the FIRST thing on its
// line. This is the part sanitizeTarget explicitly does not cover: every
// argument that comment makes for leading markers being inert rests on the
// ditto prefix putting the target in second position. A subject has no prefix,
// so all of those markers are live.
//
// Verified against the real parser (internal/content/shell_target_test.go):
// a subject of `# incident 4312` produces no node at all and the (cmd) arrow
// attaches to whatever node preceded it in the file; `- prod runbook` does the
// same; `@prod notes` silently loses the `@prod`. A sysadmin typing a ticket
// number gets a fact hanging off someone else's node.
//
// The leading marker run is bracketed rather than deleted — `# incident 4312`
// becomes `[#] incident 4312` — so the subject still says what the user typed,
// using the same bracket idiom sanitizeTarget already applies to parentheses.
//
// Position is NOT the whole story, which is the trap this originally fell into:
// ` #` opens a trailing comment at any column on a node line (see below), so
// `#` has to be handled everywhere, not just in first position. `-`, `::`, `@`,
// `"` and `'` really are leading-only — verified case by case against the real
// parser rather than reasoned about.
//
// Parentheses matter in a subject too, and for the same reason as in a target:
// `(...)` is arrow syntax wherever it appears. sanitizeTarget already handles
// that, along with tabs, newlines, trimming and the length cap, so this builds
// on it rather than repeating it.
export function sanitizeSubject(subject) {
  const s = sanitizeTarget(subject)
  if (!s) return ''
  // A space followed by `#` opens a trailing comment on a NODE line and the
  // rest of the subject is dropped — `incident #4312 - prod` reaches the graph
  // as `incident`. Unlike the leading markers this one bites anywhere in the
  // line, and unlike a target it is not neutralised by a ditto prefix (targets
  // keep their ` #` intact; verified in shell_target_test.go). Bracketing the
  // hash removes the " #" the parser scans for while keeping the character.
  let escaped = s.replace(/ #/g, ' [#]')
  escaped = escaped.replace(SUBJECT_MARKERS, (m) => `[${m}]`)
  return escaped.length > MAX_LINE ? escaped.slice(0, MAX_LINE) : escaped
}
