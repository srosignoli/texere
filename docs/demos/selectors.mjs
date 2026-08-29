// The selector contract between texere's UI and the demo plots.
//
// Plots live in a separate repository, so a rename here breaks recordings that
// texere's CI never runs. Every selector a plot may use is therefore declared
// in this one file, and selector_contract_test.js asserts each one still
// resolves against frontend/src. Rename a control and a texere test fails at
// commit time, instead of a private recording failing weeks later.
//
// Rule: plots import from here. A raw selector string in a plot is a bug —
// it is exactly the coupling this file exists to make visible.

export const SEL = {
  // --- toolbar -------------------------------------------------------------
  studyButton: 'button[title^="Study"]',
  readButton: 'button[title^="Read & import"]',
  searchButton: 'button[aria-label="Search"]',
  saveButton: 'button[aria-label="Save"]',
  explorerButton: 'button[aria-label="Open 2.5D Graph Explorer"]',

  // --- collection tree -----------------------------------------------------
  // NB: .file-row only toggles collapse. .chapter-item is what selects a
  // chapter — clicking the file name looks right and does nothing.
  chapterItem: '.chapter-item',
  chapterNamed: (title) => `.chapter-item:has-text("${title}")`,
  nodeItem: '.node-item',

  // --- study surface -------------------------------------------------------
  studyDialog: '[role="dialog"][aria-label="Study surface"]',
  recallDialog: '[role="dialog"][aria-label="Recall session"]',
  // Tabs render their label plus a due-count <sup>, so text selectors match
  // "Read5" once anything is due. Address them by aria-label.
  studyTab: (name) => `.ss-tab[aria-label="${name}"]`,
  closeStudy: 'button[aria-label="Close study"]',

  // --- reader --------------------------------------------------------------
  sourceRow: '.sb-name',
  sourceText: '.tsp',
  extract: 'button[aria-label="Extract selection"]',
  closeReader: 'button[aria-label="Close reader"]',

  // --- process -------------------------------------------------------------
  // makeCard seeds "<passage> >> " and parks the cursor for the back to be
  // typed. Clicking createCard before typing materialises nothing.
  makeCard: 'button[aria-label="Make card"]',
  createCard: 'button[aria-label="Create card"]',
  cloze: 'button[aria-label="Cloze"]',
  dismiss: 'button[aria-label="Dismiss"]',

  // --- recall --------------------------------------------------------------
  showAnswer: 'button[aria-label="Show answer"]',
  grade: (label) => `button[aria-label="${label}"]`,

  // --- editor --------------------------------------------------------------
  // Vendor-owned, see VENDOR below.
  editor: '.cm-content',
}

// Selectors emitted by a third-party library rather than applied by our own
// components. The contract test cannot verify these against frontend/src — no
// component writes them — so it exempts exactly these and no others. Keep the
// list short: every entry is a selector nothing in this repo will warn you
// about when it changes.
export const VENDOR = new Set([
  '.cm-content', // CodeMirror 6 editor content element
])
