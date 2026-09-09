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

  // --- the app's own prompt / confirm / alert -------------------------------
  // One surface for all three, driven by lib/dialog.js. It replaced
  // window.prompt / confirm / alert, which GTK drew rather than the page — so
  // they took no theme and appeared in no recording, and plots had to answer
  // them through page.on('dialog') and narrate what the viewer could not see.
  // This one is in the DOM, in the video, and typed into like anything else.
  //
  // Enter confirms and Escape cancels, matching the fingers people already
  // have. Cancel is absent on an alert, which has nothing to refuse.
  dialog: '[role="dialog"][aria-label="Dialog"]',
  // The question it is asking. Worth having declared: a plot that asserts WHICH
  // dialog opened reads this, and "some dialog appeared" is not the same claim.
  dialogTitle: '.dlg-title',
  dialogInput: 'input[aria-label="Dialog input"]',
  dialogConfirm: 'button[aria-label="Confirm dialog"]',
  dialogCancel: 'button[aria-label="Cancel dialog"]',

  // --- study surface -------------------------------------------------------
  studyDialog: '[role="dialog"][aria-label="Study surface"]',
  shortcutsDialog: '[role="dialog"][aria-label="Keyboard shortcuts"]',
  recallDialog: '[role="dialog"][aria-label="Recall session"]',
  // Tabs render their label plus a due-count <sup>, so text selectors match
  // "Read5" once anything is due. Address them by aria-label.
  studyTab: (name) => `.ss-tab[aria-label="${name}"]`,
  closeStudy: 'button[aria-label="Close study"]',
  // Which tab the surface is on. Next renders ANOTHER tab's view in its own
  // body without moving the highlight, so this stays on Next while a reader,
  // an extract or a card is on screen. That is the whole mechanic of the tab.
  studyTabActive: '.ss-tab.active',
  // What a queue shows when it has nothing left. One class, three messages,
  // told apart only by their text: "Pick a document to read.", "No extracts to
  // process." and Next's own "Nothing due" with the celebrate icon. Anything
  // waiting on it must assert the text too.
  studyEmpty: '.ss-empty',

  // --- reader --------------------------------------------------------------
  sourceRow: '.sb-name',
  sourceText: '.tsp',
  extract: 'button[aria-label="Extract selection"]',
  closeReader: 'button[aria-label="Close reader"]',

  // --- the reading list (incremental reading) ------------------------------
  // Four ways in, and only two of them a browser can drive unaided: + Text and
  // + PDF open GTK file dialogs (use DemoImportTextSource / DemoImportPdfSource
  // instead), + Web asks for a URL with window.prompt, and + LLM opens a chat
  // whose "Create document" ALSO prompts, for a title. Both prompts are
  // answerable via page.on('dialog') but the browser draws them, so neither
  // appears in the recording — narrate what the viewer cannot see.
  sourceAddText: 'button[title^="Import a text or markdown file"]',
  sourceAddPdf: 'button[title^="Import a PDF"]',
  sourceAddWeb: 'button[title^="Import a web article"]',
  sourceAddLLM: 'button[title^="Ask a model"]',
  sourceSearch: 'input[aria-label="Search sources"]',
  // Collapses the sources list. Worth having because the reading pane is a
  // 50/50 flex with no splitter, so this is the only way to give a document
  // more width — and the PDF pane renders at a fixed 1.4x with no fit-to-width,
  // so a wide page is otherwise cut off at the margin.
  sourcesHide: 'button[title="Hide sources"]',
  sourcesShow: 'button[title="Show sources"]',
  // One document in the list. The row carries its percentage read and a due
  // badge — reading elements are scheduled the same way cards are.
  sourceRow: '.sb-docrow',
  sourceDoc: '.sb-doc',
  // The clickable title of a document row — this is what OPENS it. The row
  // itself does not: the chevron beside it only unfolds the extracts, so that
  // you can open a document without expanding it.
  sourceName: '.sb-name',
  // The chevron that unfolds a document's extracts beneath it in the tree.
  // By class, not aria-label: the label is templated ('Hide extracts' when
  // open), so no literal exists in the source for the contract test to verify —
  // an entry the test cannot check is worse than a class it can.
  sourceExtractsToggle: '.sb-tri',
  // One extract, nested under its document once the chevron is open. A document
  // with none renders .sb-noextracts instead — which is what a plot expanding
  // the wrong row frames, under a caption claiming the opposite.
  sourceExtract: '.sb-extract',
  sourceNoExtracts: '.sb-noextracts',

  // The reading surface itself, and the three things you can do with what you
  // are reading. Extract turns a selection into a node; Later reschedules the
  // document; Done finishes it.
  readingBody: '.reading-body',
  // Later and Done carry no title or label of their own — they are addressed by
  // their text, through the shared .rb-btn class.
  readingBtn: '.rb-btn',
  // Per-medium panes: plain text/markdown, the PDF viewer, the fetched article.
  textPane: '.tsp',
  pdfPane: '.pdf-root',
  pdfPageLabel: '.pdf-page',
  pdfNext: '.pdf-nav',
  webArticle: '.web-article',
  openOriginal: '.web-open',

  // The ask-a-model pane: its transcript, its input, and the button that turns
  // the conversation into a document in the reading list.
  chatLog: '.lc-log',
  chatInputBox: '.lc-input',
  chatSend: '.lc-send',
  chatCreate: '.lc-create',

  // --- process: the notebook ------------------------------------------------
  // The Process tab is a split: the note FILE on the left with the scheduled
  // extract selected in it, and that extract's notebook on the right. The
  // extract is an ordinary node in the file carrying its own `# @id`, which is
  // also the key the notebook is stored under — OpenNotebook takes the node id,
  // and learning.db's notebooks table is indexed by extract_node_id. That is
  // what "the notebook is linked to the extract" means concretely, and it is
  // why reopening the extract months later brings back the reasoning that
  // produced the card.
  processLeft: '.pv-left',
  processRight: '.pv-right',
  processEmpty: '.pv-empty',

  // The notebook's cell stack, and the nine buttons that add to it. A tenth
  // block — the context — is rendered ABOVE the stack rather than added to it:
  // the extract's text, its tags, its graph neighbours (empty until Ctrl/Cmd+G
  // has been run) and the surrounding source text.
  notebookScroll: '.nb-scroll',
  notebookAddCell: '.nb-add',

  // Cell management: the notebook as a document rather than as a stack.
  //
  // FOLDING IS SUMMARISING, not hiding. cellPreview renders only while a cell
  // is folded, and every type previews from an INPUT field — a draft's first
  // line, a graph cell's query, a compute cell's first line of code, a shell
  // cell's command — so a folded notebook reads as a table of contents. Folded
  // cells also stay MOUNTED: components hold non-reactive references into the
  // cell object, so the pane mutates `folded` in place rather than replacing
  // the object.
  //
  // Fold state, order and deletion all persist through scheduleSave, so leaving
  // the tab and coming back is a fair test of any of them.
  //
  // DELETE IS IMMEDIATE. deleteCell filters and saves — no confirmation, no
  // undo. The ✕'s title is the only thing between a mis-aimed click and a lost
  // cell, which is worth saying out loud in anything that demonstrates it.
  notebookFoldAll: '.nb-foldall',
  notebookCellWrap: '.nb-cellwrap',
  notebookCellHead: '.nb-cellhead',
  notebookFold: '.nb-fold',
  notebookChevron: '.nb-chev',
  notebookCellName: '.nb-cellname',
  notebookCellPreview: '.nb-preview',
  notebookCellBody: '.nb-cellbody',
  notebookMoveUp: 'button[title="Move up"]',
  notebookMoveDown: 'button[title="Move down"]',
  notebookDeleteCell: '.nb-del',

  // Reading a notebook from the NOTE EDITOR, without opening the study surface.
  //
  // NotebookMarks parses the open file for node ids and returns a line for each
  // that has a notebook; notebookDotsGutter paints a purple dot there. Beware:
  // CodeMirror keeps SPARE gutter markers in the DOM, and they are
  // visibility:hidden — tutorial 12 found `.cm-nb-dot` elements and cut a beat
  // because every one it found was invisible. Count PAINTED dots
  // (visibility + a box + elementFromPoint), never elements.
  //
  // Clicking one arms an overlay that FOLLOWS THE CARET: it shows whatever
  // notebook the focused node has and hides when that node has none. Close is
  // sticky, so it stays dismissed until another dot is clicked.
  //
  // It opens READ-ONLY, which NotebookPane shows as a "read-only" badge and by
  // rendering none of the controls tutorial 21 covers — no add buttons, no
  // reorder, no delete. "Resume (edit)" is the deliberate escalation, and
  // moving to another node drops back to read-only.
  //
  // The dot itself. Applied from lib/n4l-editor.js rather than by a component,
  // which is why it went undeclared for a long time — the contract test walked
  // .svelte only and could not verify it. It walks lib/*.js now, so this entry
  // is checked like any other and a rename fails a texere test instead of a
  // recording weeks later.
  editorNotebookDot: '.cm-nb-dot',
  notebookOverlay: '.nb-overlay',
  notebookOverlayBar: '.nb-overlay-bar',
  notebookReadOnlyBadge: '.nb-ro',
  // The context block, pinned above the cell stack — not a cell, and not
  // addable or deletable. Four sections assembled from three different places:
  // Extract comes from the note file, Source from the reading list's copy of
  // the original, and Tags and Neighbors from the GRAPH. That last part is
  // visible as a before/after: with no graph, Neighbors reads "No graph
  // neighbors — build the graph (Ctrl+G)" and the Tags section does not render
  // at all. assembleContext runs on every OpenNotebook, so the block tracks the
  // graph rather than freezing at the moment the extract was taken.
  notebookContext: '.ctx-cell',
  notebookContextBadge: '.ctx-badge',

  // The N4L draft cell: a small editor built on the SAME n4lBaseExtensions the
  // note files use, so it has the same highlighting, the same arrow completion
  // and the same live arrow lint. Push inserts its text at the note editor's
  // cursor and records a notebook link, which is what puts a provenance dot in
  // the gutter beside the pushed node.
  //
  // The lint underline on an unregistered arrow. Applied from lib/n4l-editor.js,
  // and declared here for the same reason editorNotebookDot is: the contract
  // test now reads that file too.
  editorArrowUnregistered: '.cm-arrow-unreg',
  draftCell: '.draft-cell',
  draftPush: 'button[title^="Insert this N4L"]',

  // The graph cell: the query language of tutorial 07, inside the notebook.
  // Its input carries the same directive autocomplete as the omnibar (it shares
  // getSuggestions and the live chapter/context/arrow hints), and it has two
  // views — Text renders the result as a list, Graph renders the same result as
  // an isometric scene in the cell. Expand hands the query to the full explorer
  // by setting explorerQuery and explorerOpen, so the cell and tutorial 08's
  // surface are the same query seen at two sizes.
  //
  // .node — one result row — is deliberately not declared: seven components
  // apply that class, so as a contract entry it would name all of them. A plot
  // counts rows inside the declared results root instead.
  graphCell: '.gq-cell',
  graphCellQuery: '.gq-input',
  graphCellRun: '.gq-run',
  graphCellSuggestions: '.gq-sug',
  graphCellViews: '.gq-view',
  graphCellExpand: 'button[title^="Open in full explorer"]',
  graphCellResults: '.gq-results',
  graphCellScene: '.gq-graph',

  // The sketch cell. Strokes are drawn with real pointer events onto a canvas,
  // so a plot draws by pressing and moving the mouse — and asserts by counting
  // PAINTED PIXELS in the canvas, which is the one check in this whole contract
  // that cannot be fooled by an element that exists but renders nothing.
  //
  // Transcribe bakes the drawing to an image, sends it to a multimodal model,
  // and inserts the result as a new N4L DRAFT CELL below the sketch
  // (NotebookPane: onInsert -> insertAfter(i, newDraftCell(text))). So the
  // sketch cell hands off to the cell tutorial 12 covers, and a handwritten
  // note can end up pushed into the note file.
  sketchCell: '.sk-cell',
  sketchToolbar: '.sk-bar',
  sketchSurface: '.sk-wrap',
  sketchSize: '.sk-size',
  sketchSwatch: '.sw',
  sketchExpand: 'button[title^="Expand to fill the window"]',
  transcribeBar: '.tb',

  // The media cell. Searches openly-licensed providers — Wikimedia Commons,
  // Openverse, the Internet Archive, PeerTube and Lingua Libre — over the live
  // network, so a plot must assert that SOME result arrived, never which one.
  //
  // Image and audio are DOWNLOADED into the collection's media/ directory
  // (FetchMediaURL -> media.FetchRemote) and the note line points at
  // "media/<asset>". Video is never stored: pick() resolves a play URL and the
  // line carries that URL, so a video reference needs the network again and an
  // image does not. That asymmetry is the cell's central fact.
  //
  // Promote builds the line with buildMediaFact and inserts it as a new N4L
  // DRAFT CELL below (NotebookPane: onInsert -> insertAfter(i, newDraftCell)),
  // so this cell hands off to the one tutorial 12 covers — same as the sketch.
  //
  // The add button is labelled "+ Media", NOT "+ Media cell" like its
  // neighbours: matching add-cell buttons on exact text will miss it.
  mediaCell: '.me-cell',
  mediaLicense: '.me-lic',
  mediaSubject: '.me-subj',
  // Addressed structurally: it is the last button in the tool row. Neither its
  // text nor its title is a fixed string — both carry the arrow code for the
  // current kind ("Promote to N4L (pic)" / "(pron)" / "(vid)"), which is the
  // point of the control and changes as a tutorial switches kinds. A title
  // prefix would also sit here unverified, since the contract test can only
  // check titles that are literals in the component.
  mediaPromote: '.me-tools .me-btn:last-of-type',
  mediaSourceLink: '.me-src-btn',

  // The search panel, shown while the cell has no asset.
  mediaSearchPanel: '.ms-panel',
  // Image / Audio / Video. Scoped to .ms-kinds: `.ms-btn` alone also matches
  // Search, Import file… and Fetch.
  mediaKind: '.ms-kinds .ms-btn',
  mediaKindOn: '.ms-kinds .ms-on',
  // Only rendered for audio — the pronunciation provider takes "language|word".
  mediaLanguage: '.ms-kinds select',
  mediaProvider: '.ms-bar select',
  mediaQuery: '.ms-bar .ms-q',
  mediaSearch: '.ms-bar .ms-btn',
  mediaNote: '.ms-note',
  mediaError: '.ms-err',
  // Three result layouts for three kinds: a grid of thumbnails for images, a
  // grid with durations for video, and rows with a play button for audio.
  mediaHit: '.ms-hit',
  mediaVideoHit: '.ms-vhit',
  mediaRow: '.ms-row',
  mediaPlay: '.ms-play',
  mediaPick: '.ms-pick',

  // The three views, once an asset is on the cell. Only the image one is
  // croppable; only the video one carries a start time, because only video has
  // a moment worth marking.
  mediaImage: '.mv-view img',
  mediaCropHint: '.mv-hint',
  mediaAudioPlayer: '.av-player',
  mediaVideoPlayer: '.vv-player',
  mediaSetStart: 'button[title^="Mark the moment"]',
  mediaStartTime: '.vv-time',

  // The shell cell. Runs one command in a shell discovered on this machine and
  // streams its output into a plain-DOM pane — no xterm, so the browser's own
  // selection is the whole mechanism.
  //
  // What it promotes is a SELECTION, not the run: readSelection intersects the
  // live range with each .sh-line, and buildShellFact pairs those lines with
  // the command that produced them under the (cmd) and (out) arrows. Editing
  // the command or re-running CLEARS the selection on purpose — a fact naming
  // the wrong command is a wrong fact that looks right — so .sh-promote exists
  // only while a selection stands, which makes it the signal to assert on.
  //
  // The scrollback is component-local and NEVER persisted: leave the tab and
  // the output is gone while the command and subject survive. That is the
  // reason the cell has a promote button at all.
  shellCell: '.sh-cell',
  shellPicker: '.sh-shell',
  shellCwd: '.sh-cwd',
  shellCommand: '.sh-cmd',
  shellRun: '.sh-run',
  shellStop: '.sh-stop',
  shellOutput: '.sh-out',
  shellLine: '.sh-line',
  // stderr is a modifier on the line, applied with class:err — which is why the
  // pane can show a failure without the run being treated as one.
  shellStderr: '.sh-line.err',
  // "exit 0 · 0.4s", "cancelled", or a spawn error. A non-zero exit is
  // information, not a failure: it lands here, styled .bad, not in a toast.
  shellStatus: '.sh-status',
  shellTruncated: '.sh-trunc',
  shellPromote: '.sh-promote',
  shellCount: '.sh-count',
  shellSubject: '.sh-subj',
  shellToN4L: '.sh-n4l',
  shellWarn: '.sh-warn',

  // The LLM cell. Every selector here is SCOPED to .llm-cell on purpose: the
  // component's own class names are .skill, .prof, .msg, .body, .transcript,
  // .composer, .send and .stop, which are far too generic to address globally.
  //
  // What makes it a notebook cell rather than a chat window is what goes into
  // the system prompt. buildSystem prepends the chosen SKILL's body (built-in
  // markdown in internal/skills/builtin) and then a context block assembled
  // from the notebook: the extract, its tags, its graph neighbours and the
  // source passage — tutorial 11's context panel, fed to the model — plus the
  // text of any ATTACHED cells, so a draft, a graph result, a compute cell or a
  // baked sketch can be handed to it by ticking a checkbox.
  //
  // Unlike the shell cell's scrollback, the conversation IS persisted: finish()
  // appends the reply to cell.messages and saves. A failed turn is rolled back
  // instead — the dangling user message is removed and its text restored to the
  // composer, so a transcript never ends on an unanswered question.
  //
  // The cell needs a profile: with no active profile in config.json,
  // providerFor("") resolves nothing and the turn fails, so a plot picks one
  // from .prof before sending.
  llmCell: '.llm-cell',
  llmWarn: '.llm-cell .llm-warn',
  llmSkillPicker: '.llm-cell .skill',
  llmProfilePicker: '.llm-cell .prof',
  llmAttachBar: '.llm-cell .attach',
  llmAttachLabel: '.llm-cell .attach label',
  llmTranscript: '.llm-cell .transcript',
  llmMessage: '.llm-cell .msg',
  llmUserMessage: '.llm-cell .msg.user',
  llmAssistantMessage: '.llm-cell .msg.assistant',
  llmMessageBody: '.llm-cell .msg .body',
  // Shown while a turn streams, next to the partial reply.
  llmCaret: '.llm-cell .caret',
  llmComposer: '.llm-cell .composer textarea',
  llmSend: '.llm-cell .send',
  llmStop: '.llm-cell .stop',
  llmError: '.llm-cell .llm-error',
  llmHint: '.llm-cell .llm-hint',

  // The scan cell — the notebook's route from paper into N4L.
  //
  // "+ Scan" is ImageCell: NotebookPane.addImage calls ImportImage(), a native
  // GTK dialog, so a plot ingests through DemoImportImage(path) and adds the
  // cell itself. Only jpg/png/gif/webp are accepted; a PDF is not an option.
  //
  // Its Transcribe is the SAME TranscribeBar the sketch cell uses (SEL.transcribeBar),
  // sending the picture under TRANSCRIBE_SYSTEM — which asks for faithful text
  // and, for any diagram, a fenced ```mermaid block. The result arrives as an
  // N4L draft cell. NOTHING IN THE APP RENDERS MERMAID: the model produces the
  // description, and it stays text, so a tutorial must not imply a diagram
  // appears.
  //
  // bakeImage bakes cell.rotation and cell.crop into what it sends, so a scan
  // straightened with Rotate reaches the model upright.
  scanCell: '.im-cell',
  scanBar: '.im-cell .im-bar',
  scanRotateLeft: 'button[title="Rotate left"]',
  scanRotateRight: 'button[title="Rotate right"]',
  scanCrop: 'button[title="Crop"]',
  scanReset: 'button[title="Reset rotation and crop"]',
  scanImage: '.im-view img',
  scanCropHint: '.im-hint',
  scanMissing: '.im-missing',

  // The audio cell — a spoken thought, and the text it comes back as.
  //
  // It has TWO ways in and only one of them can be scripted. Record goes
  // through MediaRecorder and a real capture device; Import opens a native GTK
  // dialog, so a plot ingests with DemoImportAudio and appends the cell with
  // SaveNotebook, exactly as the scan cell does.
  //
  // Transcription is a SEPARATE provider from the LLM cell's: TranscribeAudio
  // resolves against cfg.STT, its own profile list, so an app with a working
  // LLM profile can still show a disabled Transcribe and the "Configure a
  // model" hint. Check the STT config before planning a take around it.
  //
  // An empty cell offers Record and Import; a filled one replaces them with the
  // duration, a player, Replace, and the transcribe row. Those are two
  // different DOM shapes, not one shape with disabled controls.
  audioCell: '.au-cell',
  audioEmpty: '.au-empty',
  audioRecord: '.au-rec',
  audioDuration: '.au-dur',
  audioPlayer: '.au-player',
  audioMissing: '.au-missing',
  audioTranscribeRow: '.au-trans',
  audioSttPicker: '.au-trans select',
  audioTranscribe: '.au-trans button',
  audioSttHint: '.au-hint',
  audioError: '.au-cell .au-err',

  // The compute cell: the user's code, run in a fresh Web Worker per run with
  // `math` (mathjs), `graph`, `chart`, `print` and a console shim in scope.
  //
  // What makes it more than a calculator is `graph.query` / `graph.orbit`,
  // which the host bridges to SearchGraph over postMessage — so code in a
  // notebook can compute over the user's OWN notes and chart the answer.
  //
  // The worker is terminated after 5s, and a run that hits that reports
  // "Timed out after 5000ms (terminated)." in .cc-error rather than hanging.
  //
  // Under `wails dev` this cell needs frontend/vite.config.js to name mathjs in
  // optimizeDeps.include AND to allow BigInt in the pre-bundler: mathjs is
  // imported only from a worker, which the dev scanner does not traverse. Left
  // out, the dep URL answers 504 and the cell says "compute worker failed" —
  // which looks like a broken plot and is not one.
  computeCell: '.cc-cell',
  computeCode: '.cc-code',
  computeRun: '.cc-run',
  computeStop: '.cc-stop',
  computeOutput: '.cc-out',
  computeLog: '.cc-log',
  computeValue: '.cc-value',
  computeError: '.cc-error',
  computeChart: '.cc-chart',

  // --- process -------------------------------------------------------------
  // makeCard seeds "<passage> >> " and parks the cursor for the back to be
  // typed. Clicking createCard before typing materialises nothing.
  // The bar swaps while a card is being seeded: Make card / Cloze / Later /
  // Dismiss are replaced by a hint and Create card. .pv-anchor carries the
  // extract's (from) anchor normally, and that hint while seeding.
  processBar: '.pv-bar',
  processAnchor: '.pv-anchor',
  // Where a refused promotion is reported, e.g. promoting to cloze with no
  // {{deletion}} marked: "mark a deletion with {{double braces}} before making
  // a cloze".
  processError: '.pv-error',
  processEditor: '.pv-editor',
  makeCard: 'button[aria-label="Make card"]',
  createCard: 'button[aria-label="Create card"]',
  cloze: 'button[aria-label="Cloze"]',
  dismiss: 'button[aria-label="Dismiss"]',

  // --- settings --------------------------------------------------------------
  // The gear opens a popover with the theme choices; a button inside it opens
  // the LLM / Speech-to-text / Skills modal.
  //
  // THE API KEY FIELD IS type="password", so what is typed renders as dots. The
  // env-var field beside it is NOT, which is how a key pasted into the wrong
  // box ends up legible on screen — see texere's 082-no-key-echo.
  //
  // A take that opens this modal shows whatever profiles the machine has. Point
  // XDG_CONFIG_HOME at a throwaway directory holding only collectionDir, and
  // the app starts with no profiles and no user skills: nothing private can
  // reach the frame, and the tutorial gets the empty state it wants anyway.
  //
  // Do NOT click Test. It makes a live request on whatever key is configured.
  settingsToggle: '.sm-toggle',
  settingsPopover: '.sm-popover',
  settingsThemeChoice: '.sm-choice',
  settingsLaunch: '.sm-launch',
  settingsModal: '.ov-modal',
  settingsTabs: '.ov-tabs',
  settingsProfileList: '.ov-list',
  settingsForm: '.ov-form',
  settingsPresets: '.ov-presets',
  settingsActions: '.ov-actions',
  settingsTestResult: '.ov-test',
  skillRow: '.skill-row',
  skillsPath: '.skills-path-value',

  // --- statistics -----------------------------------------------------------
  // The Stats tab is the only place the SCHEDULE becomes visible. CardView
  // carries no due date, so nothing in Recall can show an interval; what the
  // panel has is a per-day forecast of how many cards fall due, and a bar chart
  // of which grades were pressed.
  //
  // Both are Chart.js CANVASES — their labels and values cannot be read from
  // the DOM, so assert them through GetStatistics() and use these selectors
  // only to put the right chart on screen. Section order is: Learning quality
  // (grade mix, then difficulty), Workload (forecast), Habit, Pipeline health.
  statsPanel: '.sp-root',
  statsSection: 'section.sp-card',
  statsChart: '.sp-chartbox',
  statsStat: '.sp-stat',
  statsLabel: '.sp-label',
  statsValue: '.sp-value',

  // --- recall --------------------------------------------------------------
  showAnswer: 'button[aria-label="Show answer"]',
  grade: (label) => `button[aria-label="${label}"]`,

  // The card itself. The back is hidden until Show answer (or Space), so a
  // plot that frames the answer must reveal it first.
  //
  // Cards come from LINES IN A NOTE FILE. A node marked "# @card" splits on the
  // first ">>" into front and back; "# @card reverse" emits the swapped card
  // too; "# @cloze" emits one card per {{double-brace}} group, each blanked as
  // "[…]". A node with no "# @id" is skipped — ids are injected by
  // learning.Sync, which StudyMode runs on mount, so opening Study is what
  // materialises a card just written.
  //
  // Media works on EITHER SIDE: RecallCard runs mediaTokens() over front and
  // back independently, and n4l-media.js decides what is media by the file
  // EXTENSION rather than by the arrow's name.
  recallCard: '.rc-card',
  // The four grade buttons live here once the back is revealed.
  recallGrades: '.rc-grades',
  recallFront: '.rc-front',
  recallBack: '.rc-back',

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
  // "No results", or "Loading collection…" while the file cache fills. A query
  // that found nothing is a THIRD outcome beside results and unavailability,
  // and a plot waiting only for the first two hangs on it.
  omnibarEmpty: '.omnibar-empty',

  // --- the 2.5D graph explorer ---------------------------------------------
  // A full-surface dialog over the workbench. It is a pure CONSUMER of the
  // graph the build made: no graph, no explorer. Its launcher is also disabled
  // until a chapter is selected ("Select a chapter to enable"), so a plot opens
  // one first.
  //
  // It renders through WebGL (three.js). That works in the recording browser
  // via ANGLE/SwiftShader — software rasterisation, no GPU — which is worth
  // knowing before a plot is written around it, and worth re-checking if the
  // canvas ever comes back blank.
  explorerRoot: '[role="dialog"][aria-label="Graph Explorer"]',
  explorerCanvas: 'canvas[aria-label="Graph Explorer canvas"]',
  explorerClose: 'button[aria-label="Close Graph Explorer"]',

  // The explorer has its OWN always-visible query bar, in the same language as
  // the omnibar — which is why Ctrl/Cmd+Shift+K focuses this bar instead of
  // opening the omnibar while the explorer is up. It submits on Enter.
  explorerQueryBar: 'input[aria-label="Graph query"]',
  explorerRunQuery: 'button[aria-label="Run query"]',

  // Layers & filters. LAYERS are the context territories (checkbox, node count
  // and a solo button each); ARROWS are the four SSTorytime meta-types;
  // DISPLAY carries the stacked/extended layout toggle, the spacing and
  // opacity sliders, and Reset view.
  explorerControlsToggle: 'button[aria-label="Toggle explorer controls"]',
  explorerControls: 'aside[aria-label="Explorer controls"]',
  explorerLayerRow: '.row',
  explorerSolo: '.solo-btn',
  explorerLayoutMode: '.mode-btn',
  explorerResetView: '.reset-btn',

  // Matches / total nodes, and the two editable numbers that re-run the query:
  // limit is how many matches to anchor on, radius how far each one expands.
  explorerStats: '[role="status"][aria-label="Graph statistics"]',
  explorerLimit: 'input[aria-label="Result limit"]',
  explorerRadius: 'input[aria-label="Orbit radius"]',

  // The four edge meta-types, keyed by style as well as colour so the legend
  // reads without relying on colour alone.
  explorerEdgeLegend: '[role="img"][aria-label="Edge color legend"]',

  // --- graph query construction --------------------------------------------
  // The suggestion dropdown the graph bar opens on '\\'. It offers directives
  // first, then — after a directive that takes one — the user's REAL chapter,
  // context and arrow names, fetched from the backend (queryHints) and
  // refreshed after every build. Addressed by its listbox role rather than the
  // class, because the label is what the contract test can verify.
  queryAutocomplete: '[role="listbox"][aria-label="Query suggestions"]',
  queryAutocompleteRow: '.qac-row',
  queryAutocompleteSection: '.qac-section',

  // The optional natural-language box above the graph bar. It renders ONLY when
  // an LLM profile is configured, is provider-agnostic (any cloud or local
  // model), and never searches: it turns a question into a query and drops it
  // in the bar, editable, so what was asked of the graph stays visible.
  askBox: 'input[aria-label="Ask a question in plain language"]',
  askSubmit: '.ask-go',

  // Result shapes only the GRAPH mode produces. Fuzzy mode has none of these:
  // a path is a chain of nodes with the arrows between them, a sequence step is
  // one numbered link of a story, and a note block is the authored note behind
  // a node.
  pathChain: '.path-chain',
  pathNode: '.path-node',
  sequenceStep: '.sequence-step',
  noteBlock: '.note-block-header',
  listingChapter: '.listing-chapter-header',
  listingContext: '.listing-context-tag',

  // --- graph build, and what it reports -------------------------------------
  // Ctrl/Cmd+G rebuilds the store under <collectionDir>/sst_data (BadgerDB)
  // and <collectionDir>/bleve. It is a FULL rebuild — the builder removes both
  // directories first — which is why an edited file does not reach the graph
  // until the next build, and why a plot can prove that by querying between
  // the two.
  //
  // The banner reads "Graph ready — N nodes, M links (Xms)" and REMOVES ITSELF
  // after 5 seconds (App.svelte's setTimeout). A beat that means to frame it
  // has a five-second window.
  graphBanner: '.graph-banner',

  // The log panel, Ctrl/Cmd+Shift+M. Where a build says what it could not do:
  // per-file parse errors carry file:line, and unknown arrow codes arrive as
  // one aggregated warning naming the code and its occurrence count.
  //
  // It is append-only across builds — fixing a file and rebuilding ADDS a
  // clean result, it does not retract the old warnings. Clearing first is what
  // makes "the warnings went away" a true statement rather than a hopeful one.
  logPanel: '.log-panel',
  logRow: '.log-row',
  // "! 4", or "empty" when nothing has been logged.
  logCounts: '.log-counts',
  logEmpty: '.log-empty',
  logClear: 'button[title^="Clear all entries"]',
  // The panel's two tabs. LOG is what the last builds reported and accumulates
  // until cleared; PROBLEMS is the linter's current answer to a different
  // question — is this file actually N4L — and is REPLACED on every run
  // (SourceTab calls runLint on save, so it tracks the file, not the build).
  panelTab: (name) => `.ptab:has-text("${name}")`,
  // One linter finding. A DEFECT means the parser produced something other
  // than what was written — the note still opens and nothing is blocked, but
  // what was stored is not what the author asked for, and .defect-remediation
  // says exactly what was lost. Undeclared arrows are deliberately NOT defects
  // and never mark a file; they live in the Vocabulary column.
  defectRow: '.defect-row',
  // A row's severity, location and text live in .sev/.loc/.msg, and a defect's
  // in .defect-kind/.defect-msg/.defect-remediation — deliberately NOT
  // declared. `.msg` belongs to four other components and `.filter` to three,
  // so as contract entries they would name several things at once. A plot
  // reads them out of a declared row in an evaluate instead.

  // --- editor --------------------------------------------------------------
  // The editor's VISIBLE PANE, and therefore the thing a plot clicks to put
  // the caret in the buffer. Vendor-owned, see VENDOR below.
  //
  // Deliberately NOT .cm-content, which was here until a beat typed into
  // nothing. .cm-content is the whole DOCUMENT: on a 72-line file its box was
  // 1426px tall starting at y=-526, so its centre — where t.click aims — landed
  // at y=187, inside the columns. The click missed the editor entirely and
  // BLURRED it, and the beat then typed into no focused element while every
  // selector still resolved. .cm-scroller is the flex pane, so it is on screen
  // by construction whatever the document's length or scroll position.
  //
  // t.focusEditor() is the safe way to use this: it clicks and then asserts
  // .cm-focused, which is the only thing that distinguishes a click that took
  // focus from one that quietly did not.
  editor: '.cm-scroller',
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
  '.cm-scroller', // CodeMirror 6 scrollable viewport — the editor's visible pane
  '.cm-tooltip-autocomplete', // CodeMirror 6 autocomplete popup, opened by +link
  '.katex', // KaTeX's own wrapper: the only evidence math was typeset rather than shown as source
])
