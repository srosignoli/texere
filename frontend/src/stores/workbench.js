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
  editorCursorLine: /** @type {number|null} */ (null),
  isParsing: false,
  nodeListScrollTarget: /** @type {number|null} */ (null),
  graphVersion: 0,
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

/** omnibarMode — null = closed, 'fuzzy' = Ctrl+K, 'graph' = Ctrl+Shift+K */
export const omnibarMode = writable(/** @type {null|'fuzzy'|'graph'} */ (null))

/** omnibarOpen — legacy alias kept for any existing imports; derive from mode. */
export const omnibarOpen = { subscribe: omnibarMode.subscribe }

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
