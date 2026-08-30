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

  // --- the three columns ---------------------------------------------------
  // Column 1 — files and chapters.
  // NB: .file-row only toggles collapse. .chapter-item is what selects a
  // chapter — clicking the file name looks right and does nothing.
  fileRow: '.file-row',
  fileNamed: (name) => `.file-row:has-text("${name}")`,
  chapterItem: '.chapter-item',
  chapterNamed: (title) => `.chapter-item:has-text("${title}")`,

  // Column 2 — context tags and sequences. Chips carry data-tag, so address a
  // chip by tag rather than by text: a sequence chip renders a ⛓ glyph before
  // its name and :has-text would still match, but the attribute says what is
  // meant. .cursor-at is the chip the caret's node is currently inside.
  contextChip: '.chip',
  contextChipNamed: (tag) => `.chip[data-tag="${tag}"]`,
  sequenceChip: '.chip.sequence',

  // Column 3 — nodes.
  // The filter bar's buttons carry no aria-label (the label is the text), so
  // they are addressed by text: All, Text, Linked, Extracts, Cards.
  nodeItem: '.node-item',
  nodeNamed: (text) => `.node-item:has-text("${text}")`,
  nodeFilterBtn: '.qf-btn',
  nodeFilter: (label) => `.qf-btn:has-text("${label}")`,
  nodeRow: '.node-row-wrap',
  nodeExpand: '.chevron-btn',
  // The chevron of a named row. `.chevron-btn` alone is the first row's, which
  // is rarely the row a beat is about.
  nodeExpandFor: (text) => `.node-row-wrap:has-text("${text}") .chevron-btn`,
  // An expanded row's arrow lines. .arrow-code holds the registry LongName —
  // "leads to" for a file that says (fwd) — which is the whole point of
  // showing an expanded node on camera.
  arrowChild: '.arrow-child',
  arrowCode: '.arrow-code',
  // Sequence nodes render as a numbered timeline instead of a plain row.
  timelineItem: '.timeline-item',
  seqNum: '.seq-num',

  // Applied to the chip in column 2 and the row in column 3 that the editor's
  // caret currently sits inside. This is the editor→columns direction of the
  // coupling, and the only on-screen evidence that it happened.
  cursorAt: '.cursor-at',

  // The horizontal drag handle between the columns and the editor. The columns
  // get 220px by default, which is not enough of them to teach from.
  bandSplitter: '[role="separator"][aria-orientation="horizontal"]',

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
