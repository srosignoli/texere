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
  // inverting in Light. (#e0b000, EditCard's other literal, is NOT
  // allowlisted — it was bare chrome text at 1.77:1 in Light, per the Task 5
  // ruling; it now reads var(--stat-high) instead, see EditCard.svelte.)
  'components/explorer/EditCard.svelte': ['rgba(10, 16, 26, 0.72)'],
  // The four non-default sketch swatches (SketchCell.svelte, Task 3, feature
  // 052). Deliberately theme-invariant: they read on both a dark and a light
  // canvas, and per-theme accents would make a saved sketch look different
  // depending on which theme it is reopened in. Only the first swatch (the
  // default pen) is themed, via the --sketch-ink token.
  'components/notebook/SketchCell.svelte': ['#f59e0b', '#7dd3fc', '#ff6b6b', '#10b981'],
  // Fallbacks for when getComputedStyle returns nothing (a test environment
  // with no real document) — same reasoning as the Theme.js entry above.
  // Documented at the top of lib/css-var.js.
  'lib/css-var.js': ['#e8eaf0', '#0b1220'],
  // Chart series colours (Task 4, feature 052) — the `color` passed to each
  // <canvas use:chart>, identifying which series it is (a bar/line's own
  // hue is data, per the brief's ruling on stats-charts.js's default). Not
  // .sp-value.ok/.low/.high any more: review fix round 1 (Critical 1) found
  // those are chrome — 15px bold badge text sitting directly on
  // --surface-study — and freezing them at these same literals left Light
  // illegible (1.4-1.7:1 contrast); they now read var(--ok)/var(--accent-
  // text)/var(--stat-high) instead (see StatsPanel.svelte and style.css).
  // #e0b000 is still the deliberately-shared "amber/due" hue as a *series*
  // identity (see this task's report for the narrowed ruling: canvas series
  // and self-contained background+foreground pairs stay literal; bare text
  // on a themed surface, like the badge this used to also cover, does not).
  'components/StatsPanel.svelte': ['#7dd3fc', '#b9a3e3', '#e0b000', '#7ecfa0'],
  // '#7ecfa0' is the default series colour when a caller doesn't pass one.
  // Data, not chrome — it identifies a series, the same reasoning as
  // StatsPanel's chart colours above. '#8899aa'/'#2a3a4a' are TICK_FALLBACK/
  // GRID_FALLBACK — real chrome (axis ticks/gridlines), now resolved at call
  // time via cssVar('--text-muted', …)/cssVar('--border', …), but cssVar
  // still needs a literal to fall back to under node --test (no real
  // document) — same fallback reasoning as the Theme.js/css-var.js entries
  // above.
  'lib/stats-charts.js': ['#7ecfa0', '#8899aa', '#2a3a4a'],
  // .omnibar-backdrop is a click-catcher behind the OmniBar dropdown, dimming
  // arbitrary app content beneath it — no text sits on it, so the "scrim over
  // content, stays dark in every theme" reasoning is the same as EditCard's
  // backdrop above, not the explorer's low-alpha family (which had to flip in
  // Light because it doubles as a background FOR text). arrow-meta is a
  // self-contained bg+fg pill showing an arrow's MetaType (the arrow's own
  // category, i.e. data) — 7.31:1 internal contrast, carries its own
  // legibility into any theme, same carve-out as SourcesSidebar's due tag.
  'components/OmniBar.svelte': ['rgba(0, 0, 0, 0.35)', '#e4efe9', '#345244'],
  // .sb-tag.due/.done/.llm: three self-contained bg+fg status-tag pairs
  // (amber/green/purple), documented in-file (SourcesSidebar.svelte:181-187)
  // as the shared-hue "carries its own contrast" case — #e0b000 also drives
  // StatsPanel's chart series, #b9a3e3 its purple series, #7ecfa0 is
  // byte-identical to --ok but kept coupled to its literal background so the
  // pairing's contrast can't drift the moment a theme diverges from Blue.
  'components/SourcesSidebar.svelte': ['#4a3a1a', '#e0b000', '#1a3a2a', '#7ecfa0', '#2c2340', '#b9a3e3'],
  // .palette-backdrop is the exact same click-catcher scrim as OmniBar's
  // .omnibar-backdrop above (identical literal, identical role) — same
  // reasoning.
  'components/ArrowPalette.svelte': ['rgba(0, 0, 0, 0.35)'],
  // .banner-btn.dismiss: a neutral dark chip sitting on the amber
  // .ext-mod-banner strip, documented in-file (SourceTab.svelte:568-569) as
  // a self-contained bg+fg pair kept literal so the two halves cannot drift
  // apart per theme — same reasoning as SourcesSidebar's status tags.
  'components/SourceTab.svelte': ['#44403c', '#e8eaf0'],
  // .ov-backdrop is the same click-catcher scrim as OmniBar's/ArrowPalette's
  // above — dims arbitrary app content behind the LLM settings modal, no
  // text sits on it.
  'components/LLMSettings.svelte': ['rgba(0,0,0,0.5)'],
  // .ms-dur overlays a duration badge on an arbitrary thumbnail <img> — like
  // a video player's duration chip in any app, it needs to stay legible
  // against unpredictable photo content, not the app's own themed surface,
  // so it stays a fixed black-scrim-plus-white-text pair in every theme
  // (same reasoning as the explorer's high-alpha glass-HUD family).
  'components/notebook/MediaSearchPanel.svelte': ['rgba(0,0,0,0.75)', '#fff'],
  // .im-sel is a crop/selection marquee drawn over an arbitrary image, not a
  // themed surface — same MiniMap-precedent reasoning already documented
  // in-file (ImageCell.svelte:133-136): the translucent rgba() fill and its
  // literal border are a coupled pair, and tokenizing one while the other
  // stays a fixed literal would decouple them.
  'components/notebook/ImageCell.svelte': ['#7dd3fc', 'rgba(125, 211, 252, 0.15)'],
  // .mv-sel is the same crop/selection marquee pattern as ImageCell's
  // .im-sel above, documented in-file (MediaImageView.svelte:78-80).
  'components/notebook/media/MediaImageView.svelte': ['#6ad', 'rgba(102, 170, 221, 0.2)'],
  // .vv-player's letterbox background is always black regardless of theme —
  // the standard convention for a video element's own background (no
  // coupled text), documented in-file (MediaVideoView.svelte:116-118).
  'components/notebook/media/MediaVideoView.svelte': ['#000'],
}

// Files not yet converted. Shrinks to nothing across Tasks 2-6; when it is
// empty, Task 6 deletes it and the constant below.
const PENDING = [
  'components/AskBox.svelte',
  'components/EmptyState.svelte',
  'components/Splitter.svelte',
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
