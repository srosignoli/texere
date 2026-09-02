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
  undoButton: 'button[title^="Undo"]',

  // --- note-authoring toolbar --------------------------------------------
  // Present only while activeTab is 'note'. Titles, not aria-labels: these
  // carry their label as text (+file, +node, +ctx, +link) and the title is
  // what says which is which.
  //
  // NB: +file asks for the name with window.prompt(). A browser dialog IS
  // reachable from Playwright (page.on('dialog')) — unlike the GTK dialogs —
  // but it is drawn by the browser, not the page, so it does NOT appear in the
  // recording. A plot must both answer it and narrate it.
  newFile: 'button[title^="New .n4l file"]',
  // Inserts "\n\n" after the current line. It makes room for a node; it does
  // not write one.
  addNode: 'button[title^="Insert node"]',
  // Inserts "\n:: " and parks the cursor after it — the OPENING delimiter
  // only. Canonical N4L closes it, so a plot types the closing "::" itself.
  addContext: 'button[title^="Insert context line"]',
  // Inserts "(" and opens the arrow autocomplete. The completion writes
  // "(code) " when one is chosen; nothing is inserted if it is dismissed.
  addLink: 'button[title^="Insert arrow"]',
  explorerButton: 'button[aria-label="Open 2.5D Graph Explorer"]',

  // --- the three columns ---------------------------------------------------
  // Column 1 — files and chapters.
  // NB: .file-row only toggles collapse. .chapter-item is what selects a
  // chapter — clicking the file name looks right and does nothing.
  fileRow: '.file-row',
  fileNamed: (name) => `.file-row:has-text("${name}")`,
  chapterItem: '.chapter-item',
  chapterNamed: (title) => `.chapter-item:has-text("${title}")`,
  // Shown on the SELECTED chapter's row while the buffer is dirty. Nothing
  // marks an unselected chapter, so a plot must open one before expecting it.
  dirtyDot: '.dirty-dot',

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
  // The other half of an expanded arrow line. A media path lives here, as
  // text, which is what makes the Preview panel's rendering of the same path
  // a difference a viewer can see rather than take on trust.
  arrowTarget: '.arrow-target',
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
  shortcutsDialog: '[role="dialog"][aria-label="Keyboard shortcuts"]',
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

  // --- bottom band tabs ----------------------------------------------------
  // The two tabs over the bottom band: Note holds the CodeMirror editor on the
  // raw .n4l, Preview renders the selected chapter's parsed nodes as cards.
  // Switching to Preview is what puts formatted N4L on camera.
  //
  // Addressed by aria-label rather than by text. The labels are literal, so the
  // contract test verifies them — a text selector would sit here unverified,
  // which is how the tab formerly labelled "Read" was renamed without this file
  // noticing.
  //
  // NB: "Read" belongs to the reading workflow, not to this band — SEL.readButton
  // opens sources, and SEL.studyTab('Read') is the study surface's own tab. None
  // of the three is a way to reach the other two.
  tabBtn: '.tab-btn',
  noteTab: '.tab-btn[aria-label="Note"]',
  previewTab: '.tab-btn[aria-label="Preview"]',

  // --- the preview surface -------------------------------------------------
  // What Preview renders that no other surface does. Column 3 shows node text
  // through {node.Text} — plain, escaped, literal — so a path stays a path and
  // `\(R = e^{-t/S}\)` stays backslashes there. These selectors exist to
  // assert the difference rather than leave it to the eye.
  previewCard: '.node-card',
  // A referenced asset, rendered in the card. The element is chosen by
  // InlineMedia from the path's EXTENSION, not from the arrow's name, so the
  // tag is what says which kind was detected — hence the tag-qualified pair.
  // The class half of each is verified by the contract test through
  // inlineMedia; the tags are img/video as InlineMedia writes them.
  inlineMedia: '.inline-media',
  inlineImage: 'img.inline-media',
  inlineVideo: 'video.inline-media',
  // One typeset expression. KaTeX-owned, see VENDOR below: MathText marks only
  // FAILED typesetting with a class of ours (.math-error), so success has no
  // selector but the library's own.
  mathRendered: '.katex',
  mathError: '.math-error',
  // An all-caps node, which NodeCard renders as an amber reminder. Combine it
  // with previewCard ('.node-card' + '.reminder') rather than writing the pair
  // as one string: each half is then verified against the components, which a
  // compound selector is not.
  reminderCard: '.reminder',

  // --- the search bar ------------------------------------------------------
  // One panel with two modes, and they do not share an engine. Ctrl/Cmd+K is
  // FUZZY: a subsequence scorer (OmniBar.svelte's fuzzyScore) run in the page
  // over every .n4l the collection holds, parsed on open. Ctrl/Cmd+Shift+K is
  // GRAPH: SearchGraph, against the SSTorytime-KV store — which does not exist
  // until Ctrl/Cmd+G builds it.
  //
  // That asymmetry is a fact a plot can assert rather than narrate: on a
  // collection with no graph, fuzzy returns results and graph returns
  // omnibarGraphUnavailable. Deliberately NOT named searchPanel — searchButton
  // (the toolbar) and searchField (the editor's Ctrl/Cmd+F) are already taken,
  // and none of the three reaches the other two.
  omnibarPanel: '[role="dialog"][aria-label="Search"]',
  omnibarInput: '.omnibar-input',
  // Reads "Fuzzy" or "Graph" — the only on-screen statement of which engine is
  // about to answer.
  omnibarBadge: '.mode-badge',
  // "2 chapters · 1 node". Fuzzy searches three things and counts them
  // separately, so this is where "across the collection" becomes a number.
  omnibarSummary: '.omnibar-summary-counts',
  // The bucket headings above each group: Chapters, Contexts, Nodes.
  omnibarGroup: '.result-label',
  omnibarResult: '.result-item',
  omnibarResultText: '.result-text',
  // The right-hand column of a result row: the FILE for a chapter hit, the
  // chapter for a context or node hit. The file names are what show a viewer
  // that one query reached several files.
  omnibarResultHint: '.result-hint',
  // The arrow-key cursor. Combine with omnibarResult ('.result-item' +
  // '.selected') rather than writing the pair as one string: each half is then
  // verified against the components, which a compound selector is not.
  omnibarSelected: '.selected',
  // Graph mode with no graph: "Build your graph first (Ctrl+G)…". Its presence
  // is the proof that the fuzzy results came from somewhere else.
  omnibarGraphUnavailable: '.graph-unavailable',

  // --- editor --------------------------------------------------------------
  // Vendor-owned, see VENDOR below.
  editor: '.cm-content',
  // Rebuilds the editor, which resets the caret to the top of the document.
  vimToggle: '.vim-btn',
  // The arrow autocomplete +link opens. Vendor-owned, see VENDOR below.
  arrowPicker: '.cm-tooltip-autocomplete',
  // The Ctrl/Cmd+F panel's query field. An attribute selector rather than
  // @codemirror/search's .cm-search class, deliberately: the class would need
  // a VENDOR exemption, and that list is the contract's own warning light.
  searchField: 'input[main-field]',
}

// Selectors emitted by a third-party library rather than applied by our own
// components. The contract test cannot verify these against frontend/src — no
// component writes them — so it exempts exactly these and no others. Keep the
// list short: every entry is a selector nothing in this repo will warn you
// about when it changes.
export const VENDOR = new Set([
  '.cm-content', // CodeMirror 6 editor content element
  '.cm-tooltip-autocomplete', // CodeMirror 6 autocomplete popup, opened by +link
  '.katex', // KaTeX's own wrapper: the only evidence math was typeset rather than shown as source
])
