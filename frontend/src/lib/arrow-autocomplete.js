/**
 * arrowCompletion — CM6 CompletionSource for N4L arrow codes.
 *
 * Activates on `(` or `(\w*` patterns (user typing an arrow code inside parens).
 * Returns completions grouped by MetaType, with recently-used arrows first.
 *
 * @param {Array} arrowDefs      - Array of ArrowDef from workbenchStore.arrowDefs
 * @param {string[]} recentCodes - LRU list of recently used arrow codes
 * @param {(code: string) => void} [onSelect] - called when a completion is applied
 * @returns {import('@codemirror/autocomplete').CompletionSource}
 */
export function arrowCompletion(arrowDefs, recentCodes = [], onSelect) {
  return (context) => {
    // Match an opening paren followed by optional word characters
    const match = context.matchBefore(/\(\w*/)
    if (!match || (match.from === match.to && !context.explicit)) return null

    const recentSet = new Set(recentCodes)

    // Build completions: recently-used first, then alphabetical by MetaType + Code
    const recentDefs = []
    const otherDefs = []

    for (const def of arrowDefs) {
      if (recentSet.has(def.Code)) {
        recentDefs.push(def)
      } else {
        otherDefs.push(def)
      }
    }

    // Sort recent in LRU order
    recentDefs.sort(
      (a, b) => recentCodes.indexOf(a.Code) - recentCodes.indexOf(b.Code)
    )

    const toCompletion = (def, section) => ({
      label: def.Code,
      detail: def.LongName,
      section,
      apply: (view, completion, from, to) => {
        // Replace from the '(' through current position with "(code) "
        const insert = `(${def.Code}) `
        view.dispatch({
          changes: { from: match.from, to, insert },
          selection: { anchor: match.from + insert.length },
        })
        onSelect?.(def.Code)
      },
    })

    const completions = [
      ...recentDefs.map((d) => toCompletion(d, 'Recent')),
      ...otherDefs.map((d) => toCompletion(d, d.MetaType || 'Other')),
    ]

    return {
      from: match.from + 1, // position after '('
      options: completions,
    }
  }
}
