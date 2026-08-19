import { test } from 'node:test'
import assert from 'node:assert/strict'
import { SHORTCUT_GROUPS, KEYMAPS } from './shortcuts.js'

test('every group has a title and at least one item', () => {
  assert.ok(SHORTCUT_GROUPS.length > 0, 'no groups at all')
  for (const g of SHORTCUT_GROUPS) {
    assert.ok(typeof g.title === 'string' && g.title.length, 'group without a title')
    assert.ok(Array.isArray(g.items) && g.items.length, `${g.title}: no items`)
  }
})

test('every item has keys, a label, and at least one site', () => {
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      assert.ok(Array.isArray(it.keys) && it.keys.length, `${g.title}: item without keys`)
      assert.ok(typeof it.label === 'string' && it.label.length, `${g.title}: item without a label`)
      // site is an array even when there is one site: Ctrl/Cmd+S has four.
      assert.ok(Array.isArray(it.site), `${g.title}/${it.label}: site must be an array`)
      assert.ok(it.site.length > 0, `${g.title}/${it.label}: no site`)
    }
  }
})

test('no shortcut is listed in two groups', () => {
  const seen = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      const combo = it.keys.join('+')
      // Ctrl/Cmd+S legitimately appears twice — save a file, flush notes — so
      // the key is combo+label, not combo alone.
      const k = `${combo}|${it.label}`
      assert.ok(!seen.has(k), `${k} appears in both ${seen.get(k)} and ${g.title}`)
      seen.set(k, g.title)
    }
  }
})

import { readdirSync, readFileSync } from 'node:fs'

const SRC = new URL('../', import.meta.url)

// DOM handlers: `if ((e.ctrlKey || e.metaKey) && e.key === 'k')`.
const KEY = /\bkey\s*===\s*'([^']+)'/
// The leading `!` capture matters: OmniBar has `&& !e.shiftKey && e.key === 'k'`,
// and reading that negation as a REQUIRED modifier turns Ctrl+K into
// Ctrl+Shift+K — which is a different shortcut ten lines below it.
const MOD = /(!?)\s*\w+\.(ctrlKey|metaKey|shiftKey|altKey)\b/g
// CodeMirror keymaps: `{ key: 'Mod-s', run: … }`. Requiring `run:` on the same
// line is what rejects ordinary object keys — ExplorerLegend.svelte has
// `key: 'contains'`, `key: 'leadsto'` and four more that are data, not bindings.
const CM = /\bkey:\s*'([^']+)'\s*,\s*run:/g

// Source writes `(e.key === 'e' || e.key === 'E')` and CodeMirror writes 'Mod-s';
// one canonical case so the two syntaxes agree that Ctrl/Cmd+S is one shortcut.
// Source also writes the DOM literal 'ArrowLeft'/'ArrowRight', while the data
// in shortcuts.js displays the glyph '←'/'→' — fold the former to the latter
// here too, so a third representational difference doesn't read as a missing
// shortcut. (The data is the display form; the source is not — see the KEY
// regex above, which never sees 'Ctrl/Cmd' either, only ctrlKey/metaKey.)
const canonKey = (k) => {
  if (k === 'ArrowLeft') return '←'
  if (k === 'ArrowRight') return '→'
  return k.length === 1 ? k.toUpperCase() : k
}

function comboFromDom(line) {
  const mods = [...line.matchAll(MOD)]
  const k = line.match(KEY)
  if (!mods.length || !k) return null
  const req = new Set(mods.filter((m) => m[1] !== '!').map((m) => m[2]))
  const parts = []
  // ctrlKey||metaKey is the cross-platform idiom — one key, not two modifiers.
  if (req.has('ctrlKey') || req.has('metaKey')) parts.push('Ctrl/Cmd')
  if (req.has('shiftKey')) parts.push('Shift')
  if (req.has('altKey')) parts.push('Alt')
  return [...parts, canonKey(k[1])].join('+')
}

function comboFromCm(spec) {
  const bits = spec.split('-')
  const key = bits.pop()
  const parts = []
  if (bits.includes('Mod') || bits.includes('Ctrl') || bits.includes('Cmd')) parts.push('Ctrl/Cmd')
  if (bits.includes('Shift')) parts.push('Shift')
  if (bits.includes('Alt')) parts.push('Alt')
  return [...parts, canonKey(key)].join('+')
}

function sourceFiles() {
  return readdirSync(SRC, { recursive: true })
    .map(String)
    .filter((f) => (f.endsWith('.svelte') || f.endsWith('.js')) && !f.endsWith('.test.js'))
}

// Spread-imported keymaps: `...searchKeymap`, `...historyKeymap`, etc. These
// bring in a whole array of bindings from a package in one line — the KEY/CM
// regexes above look for per-key literals and structurally cannot see inside
// a spread, which is exactly how Ctrl/Cmd+F (CodeMirror's own search panel,
// opened via searchKeymap's Mod-f) went undocumented despite being spread
// right there in SourceTab.svelte. This regex finds the spread itself, not
// the bindings inside it — those are verified by hand against the installed
// package version and recorded as data in shortcuts.js, because they don't
// exist as literals anywhere in this tree to scan.
const KEYMAP_SPREAD = /\.\.\.([A-Za-z_$][\w$]*[Kk]eymap)\b/g

function keymapSpreadSites() {
  const sites = []
  for (const f of sourceFiles()) {
    readFileSync(new URL(f, SRC), 'utf8').split('\n').forEach((line, i) => {
      for (const m of line.matchAll(KEYMAP_SPREAD)) {
        sites.push({ name: m[1], file: f, line: i + 1 })
      }
    })
  }
  return sites
}

function sourceSites() {
  const files = sourceFiles()
  const sites = []
  for (const f of files) {
    readFileSync(new URL(f, SRC), 'utf8').split('\n').forEach((line, i) => {
      const d = comboFromDom(line)
      // `kind` records which regex family found this site. It exists so the
      // vacuity check below can assert on each family separately — a combined
      // floor can't tell "CM broke entirely" from "DOM alone already clears
      // it," which is exactly how a `run:` typo silently zeroed CodeMirror
      // extraction (and 3 of Ctrl/Cmd+S's 4 sites with it) and still passed.
      if (d) sites.push({ combo: d, file: f, line: i + 1, kind: 'dom' })
      for (const m of line.matchAll(CM)) {
        sites.push({ combo: comboFromCm(m[1]), file: f, line: i + 1, kind: 'cm' })
      }
    })
  }
  return sites
}

// Only MODIFIER shortcuts are compared. Unmodified list-navigation arrows,
// and unmodified Escape/Space/Enter (close log panel, close Study, reveal
// answer), are out of the spec's Scope table by design — see the comment
// above `SHORTCUT_GROUPS` in shortcuts.js for why they're absent from the
// data too. A modifier-based scan also structurally cannot see them (no
// modifier survives `comboFromDom`/`comboFromCm`), so they're narrowed out of
// `documented()` here regardless of the scope decision.
const documented = () => {
  const set = new Map()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) set.set(it.keys.join('+'), it)
  }
  return set
}

test('the scan finds the shortcuts known to exist', () => {
  const sites = sourceSites()
  // Measured on this tree: 15 DOM-handler sites, 4 CodeMirror keymap sites.
  // Asserted per family, not as one combined floor of 15 or 19: a combined
  // floor is exactly the value DOM extraction alone already clears, so it
  // cannot detect the CM family going to zero — which happened when `run:`
  // was mistyped in review and all 6 tests stayed green, silently losing
  // Ctrl/Cmd+S's CodeMirror sites (3 of its 4) along with it.
  const dom = sites.filter((s) => s.kind === 'dom').length
  const cm = sites.filter((s) => s.kind === 'cm').length
  assert.ok(dom >= 15, `found only ${dom} DOM-handler sites; the DOM scan is broken`)
  assert.ok(cm >= 4, `found only ${cm} CodeMirror keymap sites; the CM scan is broken`)
})

test('every documented shortcut exists at a site it names', () => {
  // Compares the FULL combo (modifiers included), not just the trailing key
  // letter — a bare-key substring check would pass `Build graph` pointed at
  // any file that happens to mention 'g' or 'G' anywhere, modifiers or not.
  const sites = sourceSites()
  const spreads = keymapSpreadSites()
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      if (it.keys.join('+') === 'F1') continue // added by this feature; Task 3 wires it
      const combo = it.keys.join('+')
      for (const f of it.site) {
        if (it.viaKeymap) {
          // Spread-sourced items don't exist as `key: '...', run:` literals
          // in this tree (the binding lives in the imported package) — what
          // IS checkable is that the named file actually spreads the keymap
          // the item claims to come from.
          const spreadsInFile = spreads.filter((s) => s.file === f).map((s) => s.name)
          assert.ok(
            spreadsInFile.includes(it.viaKeymap),
            `${g.title}/${it.label}: ${f} does not spread ...${it.viaKeymap} ` +
              `(found: ${spreadsInFile.map((n) => `...${n}`).join(', ') || 'none'})`,
          )
          continue
        }
        const combosInFile = sites.filter((s) => s.file === f).map((s) => s.combo)
        assert.ok(
          combosInFile.includes(combo),
          `${g.title}/${it.label}: ${f} has no site extracting ${combo} ` +
            `(found: ${combosInFile.join(', ') || 'none'})`,
        )
      }
    }
  }
})

test('every keymap spread found in source is declared in KEYMAPS', () => {
  // The drift case this guards: someone adds `...lintKeymap` (or any other
  // `...xKeymap`) to a component. Until it's declared here — documented and
  // covered below, or explicitly ruled out with a reason, the same as
  // `extraKeymap` — this fails, naming the spread and where it was found.
  const undeclared = []
  for (const s of keymapSpreadSites()) {
    if (!(s.name in KEYMAPS)) undeclared.push(`...${s.name} at ${s.file}:${s.line}`)
  }
  assert.deepEqual(undeclared, [],
    'keymap spread(s) found in source with no KEYMAPS entry in shortcuts.js — ' +
      `declare whether each is a source of documented shortcuts and why:\n${undeclared.join('\n')}`)
})

test('every KEYMAPS entry carries a real reason, not a bare allowlist', () => {
  for (const [name, info] of Object.entries(KEYMAPS)) {
    assert.ok(typeof info.documented === 'boolean', `KEYMAPS.${name}: documented must be true or false`)
    assert.ok(
      typeof info.reason === 'string' && info.reason.trim().length >= 20,
      `KEYMAPS.${name}: needs a substantive reason, not just a name`,
    )
  }
})

test('every viaKeymap item points at a KEYMAPS entry declared documented: true', () => {
  for (const g of SHORTCUT_GROUPS) {
    for (const it of g.items) {
      if (!it.viaKeymap) continue
      const info = KEYMAPS[it.viaKeymap]
      assert.ok(info, `${g.title}/${it.label}: viaKeymap '${it.viaKeymap}' has no KEYMAPS entry`)
      assert.ok(
        info.documented,
        `${g.title}/${it.label}: viaKeymap '${it.viaKeymap}' is declared documented: false in KEYMAPS`,
      )
    }
  }
})

test('every modifier shortcut in source is documented', () => {
  const docs = documented()
  const undocumented = []
  for (const s of sourceSites()) {
    if (!s.combo.includes('+')) continue // unmodified: contextual, out of scope
    if (!docs.has(s.combo)) undocumented.push(`${s.combo} at ${s.file}:${s.line}`)
  }
  assert.deepEqual(undocumented, [],
    `shortcuts in source but missing from the help panel:\n${undocumented.join('\n')}`)
})
