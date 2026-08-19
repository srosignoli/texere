// The keyboard shortcuts the help panel lists, as plain data so `node --test`
// can cover them — the test runner cannot parse .svelte, so anything testable
// lives outside a component (the same reason cell-label.js and icons.js exist).
//
// `site` is the file(s) implementing each binding. It is what makes
// shortcuts.test.js's guard possible: the guard verifies this claim against the
// source, in both directions. It is an array because one user-facing shortcut
// routinely has several implementations — Ctrl/Cmd+S is four sites, Ctrl/Cmd+Enter
// is three, because the same gesture is wired up per surface.
//
// `viaKeymap`, when present, names a CodeMirror keymap array (from KEYMAPS
// below) that a `site` file spreads (`...searchKeymap`) rather than binding as
// a `{ key: '...', run: ... }` literal. shortcuts.test.js's "site exists"
// check treats these two cases differently: for a `viaKeymap` item it checks
// the site file spreads that name, not that it contains a literal binding —
// the literal lives in the imported package, not this tree.
//
// KEYMAPS is the declared registry of every spread-imported keymap (a spread
// ending in "Keymap") the guard has found and a human decision on each: is
// it a source of documented shortcuts
// (and therefore covered by items below), or not (and why)? A NEW spread
// appearing anywhere in the tree that isn't a key here fails the guard until
// someone declares it — that's the drift case this exists to catch. This is
// deliberately not a bare allowlist: every entry must carry a reason, so
// silencing the guard means writing down why, not just adding a name.
export const KEYMAPS = {
  searchKeymap: {
    documented: true,
    reason: '@codemirror/search — search panel, find next, select occurrence, '
      + 'go to line. Documented individually in the Editor group below.',
  },
  historyKeymap: {
    documented: true,
    reason: '@codemirror/commands — undo/redo. Documented individually in the '
      + 'Editor group below.',
  },
  defaultKeymap: {
    documented: false,
    reason: '@codemirror/commands — ordinary editing conventions (Ctrl+A, '
      + 'Home/End, arrow movement, etc.). Out of scope per the "shortcuts '
      + 'worth memorising" rule: nobody looks up Ctrl+A.',
  },
  extraKeymap: {
    documented: false,
    reason: 'Not an imported library keymap — a local function parameter '
      + '(lib/n4l-editor.js: `keymap: extraKeymap = []`) carrying '
      + 'caller-supplied bindings. Those bindings are already documented '
      + 'individually at their own call sites (e.g. Ctrl/Cmd+S).',
  },
}
//
// Deliberately absent: Escape (close log panel, close Study/Read), Space/Enter
// (reveal answer). The spec's Scope table originally listed all three of
// these as in scope; this data narrows that down, and the spec's Scope table
// was amended to match. The reasoning: these are unmodified, contextual keys
// of the same kind as list navigation (arrows, Tab, Escape/Enter in pickers),
// which the spec already puts out of scope — not global or mode-defining, and
// documenting them would bury the entries that matter. This is not a guard
// limitation: shortcuts.test.js's exemption mechanism (see `F1`, exempted by
// name in the "site exists" test) could cover unmodified entries too — it
// just isn't used here, because these entries were narrowed out rather than
// added and exempted. The Editor group's 'Close search panel' Escape row
// below is not a counter-example: it closes a specific overlay opened by a
// shortcut documented right above it (Ctrl/Cmd+F), the same way this file
// would document a Ctrl+F-paired Escape even if the log-panel/Study Escapes
// stayed out — it isn't the ambient "dismiss whatever's open" key those are.
export const SHORTCUT_GROUPS = [
  {
    title: 'Global',
    items: [
      { keys: ['Ctrl/Cmd', 'G'], label: 'Build graph', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'K'], label: 'Search', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'K'], label: 'Graph query', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'S'], label: 'Toggle Study', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'E'], label: 'Open Read', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'R'], label: 'Toggle Recall overlay', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'M'], label: 'Toggle log panel', site: ['App.svelte'] },
      { keys: ['F1'], label: 'This panel', site: ['App.svelte'] },
    ],
  },
  {
    title: 'Editor',
    items: [
      { keys: ['Ctrl/Cmd', 'S'], label: 'Save file', site: [
        'components/SourceTab.svelte',
        'components/ProcessEditor.svelte',
        'components/explorer/EditCard.svelte',
      ] },
      // The eight below come from CodeMirror's searchKeymap/historyKeymap,
      // spread wholesale rather than bound as literals — see the `viaKeymap`
      // and `KEYMAPS` comments above for why `site` means "spreads this
      // keymap" here, not "contains this literal binding".
      { keys: ['Ctrl/Cmd', 'F'], label: 'Open search panel', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      // Find next is F3, not the CodeMirror default Ctrl/Cmd+G: App.svelte
      // binds Ctrl/Cmd+G globally to Build graph in the capture phase and
      // calls stopPropagation, so searchKeymap's Mod-g binding never reaches
      // the editor. F3 is unaffected by that clash and does work.
      { keys: ['F3'], label: 'Find next', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      { keys: ['Escape'], label: 'Close search panel', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      { keys: ['Ctrl/Cmd', 'D'], label: 'Select next occurrence', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      { keys: ['Ctrl/Cmd', 'Shift', 'L'], label: 'Select all occurrences', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      { keys: ['Ctrl/Cmd', 'Alt', 'G'], label: 'Go to line', viaKeymap: 'searchKeymap', site: [
        'components/SourceTab.svelte',
      ] },
      { keys: ['Ctrl/Cmd', 'Z'], label: 'Undo', viaKeymap: 'historyKeymap', site: [
        'lib/n4l-editor.js',
        'components/explorer/EditCard.svelte',
      ] },
      // Redo is not a single cross-platform combo like the rest of this
      // group: historyKeymap binds Mod-y (= Ctrl+Y outside macOS) with a
      // `mac: 'Mod-Shift-z'` override, so Windows/Linux and macOS genuinely
      // use different keys — two rows, not one 'Ctrl/Cmd' row.
      { keys: ['Ctrl', 'Y'], label: 'Redo (Windows/Linux)', viaKeymap: 'historyKeymap', site: [
        'lib/n4l-editor.js',
        'components/explorer/EditCard.svelte',
      ] },
      { keys: ['Cmd', 'Shift', 'Z'], label: 'Redo (Mac)', viaKeymap: 'historyKeymap', site: [
        'lib/n4l-editor.js',
        'components/explorer/EditCard.svelte',
      ] },
    ],
  },
  {
    title: 'Study & Read',
    items: [
      { keys: ['Ctrl/Cmd', 'E'], label: 'Extract selection', site: ['components/ReadingView.svelte'] },
      { keys: ['Ctrl/Cmd', 'S'], label: 'Save notes', site: ['components/ReadingView.svelte'] },
    ],
  },
  {
    title: 'Explorer',
    items: [
      { keys: ['Ctrl/Cmd', 'Shift', 'K'], label: 'Focus query bar', site: ['components/GraphExplorer.svelte'] },
      { keys: ['Alt', '←'], label: 'History back', site: ['components/GraphExplorer.svelte'] },
      { keys: ['Alt', '→'], label: 'History forward', site: ['components/GraphExplorer.svelte'] },
    ],
  },
  {
    title: 'Notebook cells',
    items: [
      { keys: ['Ctrl/Cmd', 'Enter'], label: 'Run cell / send', site: [
        'components/notebook/ShellCell.svelte',
        'components/notebook/LLMCell.svelte',
        'components/LLMChatPane.svelte',
      ] },
    ],
  },
]
