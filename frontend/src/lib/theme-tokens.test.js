import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'

const SRC = new URL('../', import.meta.url)

// Comments are stripped before scanning. n4l-escape.js:96 contains the prose
// "incident #4312 - prod", which is not a colour; without this the guard would
// demand its conversion forever.
function stripComments(s) {
  return s
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/[^\n]*/g, '$1')
}

const COLOUR = /(#[0-9a-fA-F]{3,8}\b|\brgba?\([^)]*\))/g

// A colour inside a shadow declaration is a depth cue, not a surface: it stays
// dark in every theme. n4l-editor.js states the rule already. Both spellings
// are matched — CSS uses box-shadow, EditorView.theme({}) uses boxShadow — and
// a quoted value is consumed whole so a match cannot run past it into a second
// declaration on the same line.
const SHADOW = /(?:box-shadow|boxShadow|text-shadow|textShadow|filter)\s*:\s*(?:'[^']*'|"[^"]*"|[^;\n]*)/gi

// Colours exempt for a reason other than being a shadow, as file -> literals.
// Every entry needs a comment saying why. An entry without one is unconverted
// work wearing a disguise.
const ALLOW = {
  // Fallbacks for when getComputedStyle returns nothing (a test environment
  // with no real document). Documented at the top of Theme.js. The 0x… values
  // beside them are Three.js hex integers, not CSS colours, and COLOUR does not
  // match them — which is correct, they are not styling anything. The last two
  // (cycleGlyph/cycleHalo) were added moving NodeMeshes.js's two literals
  // behind resolvers (Task 2, feature 052) — same fallback reasoning as the
  // other three.
  'components/explorer/Theme.js': ['#e8eaf0', '#0e0f12', 'rgba(200, 214, 230, 0.7)', '#f4b545', '#1a1306'],
  // .editcard-backdrop dims the arbitrary 3D graph scene behind the modal for
  // readability — same "scrim over content" reasoning as the rgba(0,0,0,0.6)
  // case (Task 2 brief), so it stays dark in every theme rather than
  // inverting in Light. #e0b000 is the app-wide "amber" notice hue that
  // StatsPanel/SourcesSidebar also keep literal on purpose (see the comment
  // on .editcard-ambiguous) — minting a texere-explorer-only token here would
  // drift it from that shared hue.
  'components/explorer/EditCard.svelte': ['rgba(10, 16, 26, 0.72)', '#e0b000'],
}

// Files not yet converted. Shrinks to nothing across Tasks 2-6; when it is
// empty, Task 6 deletes it and the constant below.
const PENDING = [
  'App.svelte',
  'components/ArrowPalette.svelte',
  'components/AskBox.svelte',
  'components/ColumnContextTags.svelte',
  'components/ColumnNodes.svelte',
  'components/EmptyState.svelte',
  'components/LLMSettings.svelte',
  'components/LogPanel.svelte',
  'components/MiddleBand.svelte',
  'components/NodeCard.svelte',
  'components/OmniBar.svelte',
  'components/QueryAutocomplete.svelte',
  'components/QueryHelpPanel.svelte',
  'components/ReadTab.svelte',
  'components/ReminderPanel.svelte',
  'components/SourceTab.svelte',
  'components/SourcesSidebar.svelte',
  'components/Splitter.svelte',
  'components/StatsPanel.svelte',
  'components/notebook/ImageCell.svelte',
  'components/notebook/MediaSearchPanel.svelte',
  'components/notebook/ShellCell.svelte',
  'components/notebook/SketchCell.svelte',
  'components/notebook/media/MediaImageView.svelte',
  'components/notebook/media/MediaVideoView.svelte',
  'lib/bake.js',
  'lib/sketch-render.js',
  'lib/stats-charts.js',
]

function sourceFiles() {
  return readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
}

function offenders(file) {
  const src = stripComments(readFileSync(new URL(file, SRC), 'utf8'))
  const shadows = [...src.matchAll(SHADOW)].map((m) => [m.index, m.index + m[0].length])
  const allowed = ALLOW[file] ?? []
  return [...src.matchAll(COLOUR)]
    .filter((m) => !shadows.some(([a, b]) => m.index >= a && m.index < b))
    .filter((m) => !allowed.includes(m[0]))
    .map((m) => m[0])
}

test('no literal colours outside the pending list', () => {
  const files = sourceFiles()
  // A file-walking test that stops walking passes forever while checking
  // nothing. Feature 050 demonstrated exactly that; this is the guard against it.
  assert.ok(files.length > 50, `scanned only ${files.length} files; the walk is broken`)

  const bad = []
  for (const f of files) {
    if (PENDING.includes(f)) continue
    const found = offenders(f)
    if (found.length) bad.push(`${f}: ${found.join(', ')}`)
  }
  assert.deepEqual(bad, [], `literal colours must be var(--token):\n${bad.join('\n')}`)
})

// The pending list is a worklist, not a dumping ground: an entry that no longer
// has any literal colour is stale and must be removed, or the list stops
// describing what is left to do.
test('every pending file still has something pending', () => {
  for (const f of PENDING) {
    assert.ok(offenders(f).length > 0, `${f} is clean — remove it from PENDING`)
  }
})

// Proves the comment-stripping works, so the #4312 false positive cannot come back.
test('a colour-shaped string in a comment is not a colour', () => {
  const src = stripComments(`// incident #4312 - prod\n<!-- #abcdef -->\n/* #123456 */\nx: #fedcba;`)
  assert.deepEqual(src.match(COLOUR), ['#fedcba'])
})

// Proves the shadow exemption covers both spellings.
test('shadow declarations are exempt in CSS and in JS', () => {
  const css = 'box-shadow: 0 6px 24px rgba(0,0,0,0.55); color: #ff0000;'
  const js = "boxShadow: '0 6px 24px rgba(0,0,0,0.55)', color: '#ff0000'"
  for (const src of [css, js]) {
    const shadows = [...src.matchAll(SHADOW)].map((m) => [m.index, m.index + m[0].length])
    const left = [...src.matchAll(COLOUR)]
      .filter((m) => !shadows.some(([a, b]) => m.index >= a && m.index < b))
      .map((m) => m[0])
    assert.deepEqual(left, ['#ff0000'], `shadow exemption wrong for: ${src}`)
  }
})
