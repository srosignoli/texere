// Shared CodeMirror setup for the N4L editors (main Source tab, Process editor,
// and the Reading notes pane). Previously each component reassembled the same
// extensions, theme, and recent-arrow LRU independently and had already drifted;
// this module is the single source of truth. Callers supply only their
// differences (extra keymap bindings, extra extensions, vim on/off).

import { EditorView, keymap, lineNumbers, drawSelection, highlightActiveLine, highlightActiveLineGutter } from '@codemirror/view'
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
    '&': { height: '100%', background: '#1b2636' },
    '.cm-content': { fontFamily: 'monospace', fontSize, color: '#e8eaf0', textAlign: 'left' },
    '.cm-gutters': { background: '#161f2e', borderRight: '1px solid #2a3a4a', color: '#4a5a6a' },
    '.cm-cursor': { borderLeftColor: '#7dd3fc' },
    '.cm-selectionBackground': { background: '#1e3a5a !important' },
    '.cm-activeLineGutter': { background: '#1d2e42' },
    '.cm-activeLine': { background: '#1d2e42' },
    '.cm-searchMatch': { background: '#854d0e', borderRadius: '2px' },
    '.cm-vim-panel': { background: '#1a2535', color: '#e8eaf0', borderTop: '1px solid #2a3a4a' },
    '.cm-vim-panel input': { background: 'transparent', color: '#e8eaf0', outline: 'none' },
    '.cm-tooltip.cm-tooltip-autocomplete': { background: '#1a2535', border: '1px solid #2a3a4a', borderRadius: '6px', boxShadow: '0 6px 24px rgba(0,0,0,0.55)' },
    '.cm-tooltip-autocomplete ul': { fontFamily: 'monospace', fontSize: '0.82rem' },
    '.cm-tooltip-autocomplete ul li': { color: '#b0bec5', padding: '3px 10px' },
    '.cm-tooltip-autocomplete ul li[aria-selected]': { background: '#1e3a5a !important', color: '#e8eaf0' },
    '.cm-completionLabel': { color: '#67e8f9' },
    '.cm-completionDetail': { color: '#8899aa', fontStyle: 'normal', marginLeft: '8px' },
    '.cm-completionSectionHeader': { color: '#5a7a9a', fontSize: '0.68rem', fontWeight: '600', textTransform: 'uppercase', letterSpacing: '0.06em', padding: '4px 10px 2px', background: '#141d2b' },
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
