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
// added and exempted.
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
