import { writable, derived } from 'svelte/store'

/**
 * workbenchStore — central runtime state for the editor workbench.
 *
 * Fields match data-model.md §3.
 */
export const workbenchStore = writable({
  collectionDir: /** @type {string|null} */ (null),
  summaries: /** @type {import('../../../wailsjs/go/models').content.ChapterSummary[]} */ ([]),
  selectedChapterKey: /** @type {{filePath: string, chapterTitle: string}|null} */ (null),
  parsedFile: /** @type {import('../../../wailsjs/go/models').content.ParsedFile|null} */ (null),
  activeContextFilter: new Set(),
  selectedNodeLine: /** @type {number|null} */ (null),
  activeTab: /** @type {'note'|'read'|'tutor'} */ ('note'),
  isDirty: false,
  externalModification: /** @type {string|null} */ (null),
  arrowDefs: /** @type {any[]} */ ([]),
  pendingScrollLine: /** @type {number|null} */ (null),
  // Monotonically increasing nonce bumped by every navigation that wants the
  // editor to scroll. Consumers (SourceTab) watch the nonce instead of the
  // line value alone, so two consecutive jumps to the *same* line still
  // re-trigger the scroll (Svelte reactivity dedupes identical primitives).
  pendingScrollNonce: 0,
  editorCursorLine: /** @type {number|null} */ (null),
  isParsing: false,
  nodeListScrollTarget: /** @type {number|null} */ (null),
  graphVersion: 0,
  // Monotonically increasing nonce bumped when the backend has injected # @id
  // markers into the open file and the buffer should silently reload from disk
  // (no banner, only when buffer is clean). SourceTab watches this field.
  silentReloadNonce: 0,
})

/**
 * contextTags — unique context tag names from the selected chapter's nodes.
 * Each entry: { name: string, isSequence: boolean, isActive: boolean }
 *
 * Special handling for _sequence_:
 * - Tags that co-occur with _sequence_ (e.g. "pod lifecycle") are marked
 *   isSequence:true and act as the named handle for that sequence.
 * - The bare _sequence_ chip is only shown when it appears alone (no companion
 *   tag), so it never acts as a merged view across all named sequences.
 */
export const contextTags = derived(workbenchStore, ($w) => {
  if (!$w.parsedFile || !$w.selectedChapterKey) return []

  const chapter = $w.parsedFile.Chapters?.find(
    (ch) => ch.Title === $w.selectedChapterKey.chapterTitle
  )
  if (!chapter) return []

  const seen = new Map()    // name → { coSeq: boolean }
  let hasLoneSequence = false

  for (const node of chapter.Nodes ?? []) {
    const ctx = node.EffectiveCtx ?? []
    const hasSeq = ctx.includes('_sequence_')
    const others = ctx.filter((t) => t !== '_sequence_')

    if (hasSeq && others.length === 0) hasLoneSequence = true

    for (const tag of ctx) {
      if (tag === '_sequence_') continue
      if (!seen.has(tag)) {
        seen.set(tag, { coSeq: hasSeq })
      } else if (hasSeq) {
        seen.get(tag).coSeq = true
      }
    }
  }

  const result = Array.from(seen.entries()).map(([name, meta]) => ({
    name,
    isSequence: meta.coSeq,
    isActive: $w.activeContextFilter.has(name),
  }))

  // Only show _sequence_ chip when it appears without a companion tag.
  if (hasLoneSequence) {
    result.unshift({
      name: '_sequence_',
      isSequence: true,
      isActive: $w.activeContextFilter.has('_sequence_'),
    })
  }

  return result
})

/**
 * filteredNodes — nodes from the selected chapter filtered by activeContextFilter.
 * If _sequence_ is active, nodes are sorted by Line (file order = sequence order).
 */
export const filteredNodes = derived(workbenchStore, ($w) => {
  if (!$w.parsedFile || !$w.selectedChapterKey) return []

  const chapter = $w.parsedFile.Chapters?.find(
    (ch) => ch.Title === $w.selectedChapterKey.chapterTitle
  )
  if (!chapter) return []

  const nodes = chapter.Nodes ?? []
  const filter = $w.activeContextFilter

  if (filter.size === 0) return nodes

  // Keep nodes whose EffectiveCtx overlaps activeContextFilter (OR / union):
  // a node is included if it belongs to ANY of the selected context tags.
  const filtered = nodes.filter((node) => {
    const ctx = node.EffectiveCtx ?? []
    for (const tag of ctx) {
      if (filter.has(tag)) return true
    }
    return false
  })

  // Sort by Line (file order = sequence order) when the result contains sequence
  // nodes.  This covers both the bare _sequence_ filter and named-sequence tags
  // like "pod lifecycle" that co-occur with _sequence_.
  const isSeqResult =
    filter.has('_sequence_') ||
    filtered.some((n) => (n.EffectiveCtx ?? []).includes('_sequence_'))
  if (isSeqResult) {
    return filtered.slice().sort((a, b) => a.Line - b.Line)
  }

  return filtered
})

/**
 * reminderCount — total reminders in the currently open file.
 * Cross-file aggregation is deferred to a future enhancement.
 */
export const reminderCount = derived(workbenchStore, ($w) => {
  return $w.parsedFile?.Reminders?.length ?? 0
})

/**
 * omnibarMode — null = closed, 'fuzzy' = Ctrl+K, 'graph' = Ctrl+Shift+K
 *
 * There used to be an `omnibarOpen` alias beside this — `{ subscribe:
 * omnibarMode.subscribe }`, kept "for any existing imports". It read like a
 * boolean store and wasn't one: it had no `update`, and MiddleBand's Search
 * button called `omnibarOpen.update(v => !v)`, which threw and blanked the
 * whole webview on every click. By the time that was found the alias had no
 * importers left, so it is gone rather than repaired. Toggle the mode directly.
 */
export const omnibarMode = writable(/** @type {null|'fuzzy'|'graph'} */ (null))

/**
 * shortcutsOpen — whether the keyboard-shortcuts help panel is visible.
 *
 * Same split-ownership problem as omnibarMode above: App.svelte owns the F1
 * keydown trigger, MiddleBand.svelte owns the "?" button, so the boolean
 * cannot live in either component. A plain writable (not a subscribe-only
 * alias) so both `.set()` and `.update()` work — see the omnibarMode comment
 * above for what happens when a caller assumes `.update()` exists and it
 * doesn't.
 */
export const shortcutsOpen = writable(false)

/**
 * llmSettingsOpen — the LLM / speech / skills overlay.
 *
 * Lives here rather than in either component because neither can own it:
 * SettingsMenu triggers it, App.svelte mounts it. Mounting it inside the
 * popover is not an option — SettingsMenu dismisses on `pointerdown` anywhere
 * outside its root element, so the first click *inside* a nested overlay would
 * read as "outside" and close both.
 */
export const llmSettingsOpen = writable(false)

/**
 * editorAPI — live reference to SourceTab's imperative methods.
 * SourceTab registers itself on mount and clears on destroy.
 * Any component can use $editorAPI?.triggerSave() etc. without prop-drilling.
 */
export const editorAPI = writable(/** @type {{
  triggerSave: () => void,
  triggerUndo: () => void,
  insertAtCursor: (text: string) => void,
} | null} */ (null))

/**
 * navigateToLine — single source of truth for "open this file at this line"
 * navigations triggered from search results, log-panel rows, etc. Centralises
 * the workbenchStore mutation so every caller sets the same fields and bumps
 * pendingScrollNonce, ensuring the editor scrolls even when the new line
 * equals the previous one.
 *
 * Pass `line = null` to open the file without targeting a specific line
 * (e.g. an empty newly-created file). chapterTitle may be '' when the file
 * has no chapters yet.
 */
export function navigateToLine({ filePath, chapterTitle, line, parsedFile, contextFilter }) {
  workbenchStore.update((s) => ({
    ...s,
    selectedChapterKey: { filePath, chapterTitle: chapterTitle ?? '' },
    parsedFile: parsedFile ?? s.parsedFile,
    activeContextFilter: contextFilter instanceof Set ? contextFilter : new Set(),
    selectedNodeLine: line ?? null,
    pendingScrollLine: line ?? null,
    pendingScrollNonce: (s.pendingScrollNonce ?? 0) + 1,
    nodeListScrollTarget: line ?? null,
    activeTab: 'note',
    isDirty: false,
    isParsing: false,
  }))
}
