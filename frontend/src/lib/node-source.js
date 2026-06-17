// resolveNodeSource maps a graph-explorer node identity back to the concrete
// .n4l source location the user can edit. Explorer node ids are "Chapter::Text"
// (or just "Text" when the node has no chapter), matching internal/explorer
// types.go nodeKey.id(). We recover { filePath, chapterTitle, line } by matching
// node text (and, when present, chapter) against parsed files. The chapter is a
// preference, not a filter: if no chapter+text hit exists we fall back to the
// first text-only hit, so callers that pass a bare text id keep first-match
// semantics. Shared by the explorer edit-card and the omnibar.

/**
 * @param {string} nodeId  "Chapter::Text" or "Text".
 * @param {Record<string, {Chapters?: Array<{Title?: string, Nodes?: Array<{Text?: string, Line: number}>}>}>} fileCache
 *        map of filePath -> ParsedFile (as returned by the ParseFileForUI binding).
 * @returns {{ filePath: string, chapterTitle: string, line: number, matchCount: number } | null}
 */
export function resolveNodeSource(nodeId, fileCache) {
  if (!nodeId || !fileCache) return null
  const sep = nodeId.indexOf('::')
  const chapter = sep === -1 ? '' : nodeId.slice(0, sep)
  const text = sep === -1 ? nodeId : nodeId.slice(sep + 2)
  const wanted = String(text).trim()
  if (!wanted) return null

  const matches = []
  let chapterMatch = null
  for (const [filePath, pf] of Object.entries(fileCache)) {
    for (const ch of pf?.Chapters ?? []) {
      for (const node of ch?.Nodes ?? []) {
        if (String(node?.Text ?? '').trim() !== wanted) continue
        const hit = { filePath, chapterTitle: ch.Title ?? '', line: node.Line }
        matches.push(hit)
        if (chapter && chapterMatch === null && (ch.Title ?? '') === chapter) {
          chapterMatch = hit
        }
      }
    }
  }
  if (matches.length === 0) return null
  const chosen = chapterMatch ?? matches[0]
  return { ...chosen, matchCount: matches.length }
}
