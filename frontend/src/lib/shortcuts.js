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
// (reveal answer). The spec's Scope table lists all three of these as in
// scope for the panel — this is not the spec's own out-of-scope carve-out,
// which excludes only Escape/Enter inside pickers, a different thing. They're
// left out of THIS data because shortcuts.test.js's guard only ever compares
// MODIFIER combos: an unmodified entry here would be documented and never
// checked by either direction of the guard (`F1` is the one exception,
// exempted by name in the "site exists" test). If that changes, the guard's
// `documented`/`comboFromDom` would need to widen to unmodified keys before
// entries for them belong here.
export const SHORTCUT_GROUPS = [
  {
    title: 'Global',
    items: [
      { keys: ['Ctrl/Cmd', 'G'], label: 'Build graph', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'K'], label: 'Search', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'K'], label: 'Graph query', site: ['components/OmniBar.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'S'], label: 'Open Study', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'E'], label: 'Open Read', site: ['App.svelte'] },
      { keys: ['Ctrl/Cmd', 'Shift', 'R'], label: 'Open Recall overlay', site: ['App.svelte'] },
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
