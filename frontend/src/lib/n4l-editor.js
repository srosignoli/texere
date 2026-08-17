// Shared CodeMirror setup for the N4L editors (main Source tab, Process editor,
// and the Reading notes pane). Previously each component reassembled the same
// extensions, theme, and recent-arrow LRU independently and had already drifted;
// this module is the single source of truth. Callers supply only their
// differences (extra keymap bindings, extra extensions, vim on/off).

import { EditorView, keymap, lineNumbers, drawSelection, highlightActiveLine, highlightActiveLineGutter, Decoration, ViewPlugin, hoverTooltip, gutter, GutterMarker } from '@codemirror/view'
import { StateField, StateEffect } from '@codemirror/state'
import { defaultKeymap, historyKeymap, history } from '@codemirror/commands'
import { autocompletion } from '@codemirror/autocomplete'
import { vim } from '@replit/codemirror-vim'
import { n4lLanguage } from './n4l-language.js'
import { n4lHighlighting } from './n4l-highlight.js'
import { arrowCompletion } from './arrow-autocomplete.js'

// n4lTheme is the shared editor theme. Selectors for features a given editor
// does not enable (search, vim command line, autocomplete tooltip) are harmless
// no-ops there. fontSize is the only per-editor knob.
export function n4lTheme(fontSize = '0.85rem') {
  return EditorView.theme({
    '&': { height: '100%', background: 'var(--surface)' },
    '.cm-content': { fontFamily: 'monospace', fontSize, color: 'var(--text)', textAlign: 'left' },
    '.cm-gutters': { background: 'var(--surface-bar)', borderRight: '1px solid var(--border)', color: 'var(--text-dim)' },
    '.cm-cursor': { borderLeftColor: 'var(--accent-text)' },
    '.cm-selectionBackground': { background: 'var(--accent) !important' },
    '.cm-activeLineGutter': { background: 'var(--editor-active-line)' },
    '.cm-activeLine': { background: 'var(--editor-active-line)' },
    '.cm-searchMatch': { background: 'var(--editor-search-match)', borderRadius: '2px' },
    '.cm-vim-panel': { background: 'var(--surface-overlay)', color: 'var(--text)', borderTop: '1px solid var(--border)' },
    '.cm-vim-panel input': { background: 'transparent', color: 'var(--text)', outline: 'none' },
    // boxShadow's rgba(0,0,0,0.55) is a drop shadow, not a themed colour — it
    // stays dark under the tooltip in every theme, so it is left as-is.
    '.cm-tooltip.cm-tooltip-autocomplete': { background: 'var(--surface-overlay)', border: '1px solid var(--border)', borderRadius: '6px', boxShadow: '0 6px 24px rgba(0,0,0,0.55)' },
    '.cm-tooltip-autocomplete ul': { fontFamily: 'monospace', fontSize: '0.82rem' },
    '.cm-tooltip-autocomplete ul li': { color: 'var(--text-row)', padding: '3px 10px' },
    '.cm-tooltip-autocomplete ul li[aria-selected]': { background: 'var(--accent) !important', color: 'var(--text)' },
    '.cm-completionLabel': { color: 'var(--n4l-syntax-arrow-code)' },
    '.cm-completionDetail': { color: 'var(--text-muted)', fontStyle: 'normal', marginLeft: '8px' },
    '.cm-completionSectionHeader': { color: 'var(--accent-dim)', fontSize: '0.68rem', fontWeight: '600', textTransform: 'uppercase', letterSpacing: '0.06em', padding: '4px 10px 2px', background: 'var(--editor-completion-header)' },
  })
}

// pushRecentArrow returns the LRU-updated recent-arrow-codes list (most recent
// first, deduped, capped at 10). Callers persist the result via UpdateRecentArrows.
export function pushRecentArrow(list, code) {
  return [code, ...list.filter((c) => c !== code)].slice(0, 10)
}

// n4lBaseExtensions assembles the CodeMirror extensions common to every N4L
// editor. getArrowDefs/getRecent are called at completion time so the autocomplete
// always sees the component's current (reactive) values rather than a stale
// snapshot captured at build time.
//
// opts:
//   getArrowDefs()   -> arrow definition array (fresh)
//   getRecent()      -> recent arrow codes array (fresh)
//   onArrowSelected  -> called with a code when a completion is applied
//   vim              -> enable vim keybindings (default false)
//   keymap           -> extra key bindings, tried before defaultKeymap/historyKeymap
//   extra            -> extra extensions appended last (search, updateListener, …)
//   fontSize         -> .cm-content font size (default 0.85rem)
export function n4lBaseExtensions(opts) {
  const {
    getArrowDefs, getRecent, onArrowSelected,
    vim: vimEnabled = false, keymap: extraKeymap = [], extra = [], fontSize,
  } = opts
  return [
    ...(vimEnabled ? [vim()] : []),
    lineNumbers(), drawSelection(), highlightActiveLine(), highlightActiveLineGutter(),
    history(), n4lLanguage, n4lHighlighting,
    autocompletion({ override: [(ctx) => arrowCompletion(getArrowDefs(), getRecent(), onArrowSelected)(ctx)] }),
    keymap.of([...extraKeymap, ...defaultKeymap, ...historyKeymap]),
    EditorView.lineWrapping,
    n4lTheme(fontSize),
    ...extra,
  ]
}

// Mark every (code) arrow whose code is unregistered and that has a target.
function unregisteredArrowDeco(view, getArrowDefs) {
  const codes = new Set((getArrowDefs() ?? []).map((d) => d.Code))
  const marks = []
  const re = /\(([^()\n]+)\)/g
  for (const { from, to } of view.visibleRanges) {
    const text = view.state.doc.sliceString(from, to)
    let m
    while ((m = re.exec(text))) {
      const code = m[1].trim()
      if (!code || codes.has(code)) continue
      const after = text.slice(m.index + m[0].length)
      const target = after.split(/\(|\n/)[0].trim() // arrow needs a non-empty target
      if (!target) continue
      const start = from + m.index
      marks.push(Decoration.mark({ class: 'cm-arrow-unreg' }).range(start, start + m[0].length))
    }
  }
  return Decoration.set(marks, true)
}

// Rank registered arrows by shared-prefix length, then closeness in length.
function nearestArrows(code, defs) {
  const score = (c) => {
    let p = 0
    const n = Math.min(c.length, code.length)
    while (p < n && c[p] === code[p]) p++
    return p * 100 - Math.abs(c.length - code.length)
  }
  return [...(defs ?? [])].sort((a, b) => score(b.Code) - score(a.Code))
}

// arrowLint underlines unregistered arrows and offers replacement suggestions.
export function arrowLint(getArrowDefs) {
  const decoPlugin = ViewPlugin.fromClass(
    class {
      constructor(view) { this.decorations = unregisteredArrowDeco(view, getArrowDefs) }
      update(u) { if (u.docChanged || u.viewportChanged) this.decorations = unregisteredArrowDeco(u.view, getArrowDefs) }
    },
    { decorations: (v) => v.decorations },
  )

  const tip = hoverTooltip((view, pos) => {
    const line = view.state.doc.lineAt(pos)
    const rel = pos - line.from
    const re = /\(([^()\n]+)\)/g
    let m
    while ((m = re.exec(line.text))) {
      const s = m.index
      const e = m.index + m[0].length
      if (rel < s || rel > e) continue
      const code = m[1].trim()
      const codes = (getArrowDefs() ?? []).map((d) => d.Code)
      if (!code || codes.includes(code)) return null
      const after = line.text.slice(e)
      const target = after.split(/\(|\n/)[0].trim()
      if (!target) continue // parenthetical prose, not an arrow — no tooltip
      const from = line.from + s
      const to = line.from + e
      const suggestions = nearestArrows(code, getArrowDefs()).slice(0, 5)
      return {
        pos: from,
        end: to,
        above: true,
        create() {
          const dom = document.createElement('div')
          dom.className = 'cm-arrow-fix'
          const h = document.createElement('div')
          h.className = 'cm-arrow-fix-h'
          h.textContent = `Unregistered arrow "${code}"`
          dom.appendChild(h)
          for (const sug of suggestions) {
            const b = document.createElement('button')
            b.textContent = `(${sug.Code}) ${sug.LongName || ''}`.trim()
            b.onclick = () => view.dispatch({ changes: { from, to, insert: `(${sug.Code})` } })
            dom.appendChild(b)
          }
          return { dom }
        },
      }
    }
    return null
  })

  const theme = EditorView.theme({
    '.cm-arrow-unreg': { textDecoration: 'underline wavy #f59e0b' },
    '.cm-arrow-fix': { background: '#1a2537', border: '1px solid #2a3a4a', borderRadius: '6px', padding: '6px', maxWidth: '260px' },
    '.cm-arrow-fix-h': { color: '#f59e0b', fontSize: '11px', marginBottom: '4px' },
    '.cm-arrow-fix button': { display: 'block', width: '100%', textAlign: 'left', background: 'none', border: 'none', color: '#cdd6e2', cursor: 'pointer', fontSize: '12px', padding: '2px 4px' },
    '.cm-arrow-fix button:hover': { background: '#24374d' },
  })

  return [decoPlugin, tip, theme]
}

// ----- Provenance gutter dots (notebook links) -----

const setDots = StateEffect.define() // payload: number[] of 1-indexed lines with dots
const dotsField = StateField.define({
  create() { return [] },
  update(val, tr) {
    for (const e of tr.effects) if (e.is(setDots)) return e.value
    return val
  },
})

class DotMarker extends GutterMarker {
  toDOM() {
    const el = document.createElement('span')
    el.className = 'cm-nb-dot'
    el.textContent = '●'
    return el
  }
}
const dotMarker = new DotMarker()

// notebookDotsGutter renders a purple dot in the gutter for each line in the
// dotsField. onClick receives the 1-indexed line number.
export function notebookDotsGutter(onClick) {
  return [
    dotsField,
    gutter({
      class: 'cm-nb-gutter',
      lineMarker(view, line) {
        const ln = view.state.doc.lineAt(line.from).number
        return view.state.field(dotsField).includes(ln) ? dotMarker : null
      },
      // Without this, the gutter only recomputes markers on doc/viewport
      // changes — a setDots effect (applyDots) would update dotsField silently
      // and the dot wouldn't paint until the next unrelated update (e.g. the
      // user selecting the line). This makes applyDots repaint immediately.
      lineMarkerChange: (update) =>
        update.transactions.some((tr) => tr.effects.some((e) => e.is(setDots))),
      initialSpacer: () => dotMarker,
      domEventHandlers: {
        click(view, line) {
          const ln = view.state.doc.lineAt(line.from).number
          if (view.state.field(dotsField).includes(ln)) { onClick(ln); return true }
          return false
        },
      },
    }),
    EditorView.theme({
      '.cm-nb-dot': { color: '#a855f7', cursor: 'pointer', fontSize: '10px' },
    }),
  ]
}

// applyDots dispatches new dot lines onto an existing view.
export function applyDots(view, lines) {
  view.dispatch({ effects: setDots.of(lines) })
}
