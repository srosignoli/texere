// The keyboard shortcuts the help panel lists, as plain data so `node --test`
// can cover them — the test runner cannot parse .svelte, so anything testable
// lives outside a component (the same reason cell-label.js and icons.js exist).
//
// `site` is the file(s) implementing each binding. It is what makes
// shortcuts.test.js's guard possible: the guard verifies this claim against the
// source, in both directions. It is an array because one user-facing shortcut
// routinely has several implementations — Ctrl/Cmd+S is four sites, Ctrl/Cmd+Enter
// is three, because the same gesture is wired up per surface.
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
