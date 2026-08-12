// The nodes column's quick filters.
//
// These predicates lived inside ColumnNodes.svelte, where this repo's
// `node --test` cannot reach them — which is how the old "Extracts" filter came
// to mean "no arrows and no study marker", never looking at # @id at all.
//
// The five filters are overlapping LENSES, not exclusive buckets: a node may
// match several, because "has arrows" and "was extracted" are different
// questions about the same node.
//
// Dependency-free: no Svelte, no stores. A node is the parser's shape as it
// arrives through Wails — { ID, Text, Arrows, Dittos, StudyMeta }.

/** An arrow on the node itself, or on any of its ditto continuation lines. */
export function hasArrows(node) {
  return (node?.Arrows?.length > 0) ||
         (node?.Dittos?.some((d) => d.Arrows?.length > 0) ?? false)
}

/** Prose: no N4L structure and not studyable. */
export function isText(node) {
  return !hasArrows(node) && !node?.StudyMeta
}

/** Carries at least one arrow. Named for the toolbar's +link button. */
export function isLinked(node) {
  return hasArrows(node)
}

/** Extracted from a source: has a # @id and has not been promoted to a card. */
export function isExtract(node) {
  return !!node?.ID && !node?.StudyMeta
}

/**
 * Studyable. Deliberately "has a study marker" rather than "# @id AND # @card":
 * # @cloze and # @study set StudyMeta too, and a hand-written # @card need not
 * carry an # @id — under the literal rule such a node would appear in no filter
 * at all.
 */
export function isCard(node) {
  return !!node?.StudyMeta
}

/**
 * The ROW RENDERER's switch, not a filter: does this node have anything to
 * expand? This is the old isStructured, kept verbatim. The filter bar no longer
 * uses it — Linked is the narrower hasArrows — and the two must not be merged
 * again, or a card with no arrows loses its collapsible row.
 */
export function hasDetail(node) {
  return hasArrows(node) || !!node?.StudyMeta
}

/**
 * The filter bar, in display order. One table drives both buttons and
 * filtering — frozen (array and each entry) so that invariant is enforced,
 * not just intended: a consumer that tries to push, reorder, or reassign a
 * field silently no-ops (or throws under strict-mode ESM) instead of the bar
 * quietly drifting from the filtering logic.
 */
export const NODE_FILTERS = Object.freeze([
  Object.freeze({ id: 'all',      label: 'All',      match: () => true }),
  Object.freeze({ id: 'text',     label: 'Text',     match: isText }),
  Object.freeze({ id: 'linked',   label: 'Linked',   match: isLinked }),
  Object.freeze({ id: 'extracts', label: 'Extracts', match: isExtract }),
  Object.freeze({ id: 'cards',    label: 'Cards',    match: isCard }),
])
