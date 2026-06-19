// resolveNodeSource maps a graph-explorer node identity back to the concrete
// .n4l source location the user can edit. Explorer node ids are "Chapter::Text"
// (or just "Text" when the node has no chapter), matching internal/explorer
// types.go nodeKey.id().
//
// A graph node may appear in a .n4l file in two ways:
//   - as a SUBJECT  — the text starts a node line (Chapter.Nodes[].Text) or a
//     ditto continuation line (Node.Dittos[].Text); we resolve to that line.
//   - as a REFERENCE — the text is the target of a typed arrow on some line
//     (Node.Arrows[].Target or Node.Dittos[].Arrows[].Target); we resolve to
//     the line that references it. Many graph nodes are objects that never
//     start a line, so this case is common, not an edge case.
// A subject line is preferred over a reference line when both exist (editing the
// node's own line is more natural). Within the chosen kind, the chapter from the
// node id is a preference, not a filter: if no chapter match exists we fall back
// to the first match, so callers passing a bare text id keep first-match
// semantics. Shared by the explorer edit-card and the omnibar.

/**
 * @param {string} nodeId  "Chapter::Text" or "Text".
 * @param {Record<string, {Chapters?: Array<{Title?: string, Nodes?: Array<{
 *   Text?: string, Line: number,
 *   Arrows?: Array<{Target?: string, Line: number}>,
 *   Dittos?: Array<{Text?: string, Line: number, Arrows?: Array<{Target?: string, Line: number}>}>
 * }>}>}>} fileCache  map of filePath -> ParsedFile (as returned by ParseFileForUI).
 * @returns {{ filePath: string, chapterTitle: string, line: number, matchCount: number } | null}
 */
export function resolveNodeSource(nodeId, fileCache) {
  if (!nodeId || !fileCache) return null
  const sep = nodeId.indexOf('::')
  const chapter = sep === -1 ? '' : nodeId.slice(0, sep)
  const text = sep === -1 ? nodeId : nodeId.slice(sep + 2)
  const wanted = String(text).trim()
  if (!wanted) return null

  const eq = (s) => String(s ?? '').trim() === wanted
  const subjects = [] // node/ditto text == wanted (the node's own line)
  const refs = []     // arrow target == wanted (a line that references the node)

  for (const [filePath, pf] of Object.entries(fileCache)) {
    for (const ch of pf?.Chapters ?? []) {
      const chapterTitle = ch?.Title ?? ''
      for (const node of ch?.Nodes ?? []) {
        if (eq(node?.Text)) subjects.push({ filePath, chapterTitle, line: node.Line })
        for (const arrow of node?.Arrows ?? []) {
          if (eq(arrow?.Target)) refs.push({ filePath, chapterTitle, line: arrow.Line })
        }
        for (const ditto of node?.Dittos ?? []) {
          if (eq(ditto?.Text)) subjects.push({ filePath, chapterTitle, line: ditto.Line })
          for (const arrow of ditto?.Arrows ?? []) {
            if (eq(arrow?.Target)) refs.push({ filePath, chapterTitle, line: arrow.Line })
          }
        }
      }
    }
  }

  const pool = subjects.length ? subjects : refs
  if (pool.length === 0) return null
  const chosen = (chapter && pool.find((h) => h.chapterTitle === chapter)) || pool[0]
  return { ...chosen, matchCount: pool.length }
}
