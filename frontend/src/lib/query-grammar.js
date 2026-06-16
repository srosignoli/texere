// Single source of truth for the graph query language used by both the
// OmniBar graph search (Ctrl+Shift+K) and the explorer's always-visible
// query bar. The autocomplete dropdown AND the help panel are both generated
// from the data here, so the docs and the suggestions can never drift.
//
// ⚠️ GROUNDED IN SOURCE (github.com/srosignoli/sstorytime-kv v0.5.0), not
// guessed:
//   - Directive tokens + aliases: DecodeSearchField / FillInParameters
//     keyword list, pkg/SSTorytime/SSTorytime.go (const block ~4104,
//     FillInParameters ~4249). \limit, \depth, \range, \distance all set the
//     single Range field (one shared cap). \sequence/\seq/\story/\stories are
//     the same flag; \from/\path/\paths are the same; \context/\ctx/\as are
//     the same; \chapter/\section/\in/\contents/\toc/\map are the same.
//   - Routing precedence: texere internal/search/search.go classifyQuery.
//   - Matching semantics: pkg/SSTorytime/bleve_translate.go + its tests
//     (bleve_translate_test.go, bleve_index_test.go) — every operator below
//     is pinned by a passing test (stemming, AND &, OR |, NOT !, exact !x!/|x|,
//     prefix x:*, phrase "...", proximity <->/<N>, accent-fold (x), CJK bigram).

/** @typedef {{token:string, aliases:string[], arg:string, valueKind:string, summary:string, example:string}} Directive */

/**
 * valueKind drives which live values (if any) the autocomplete offers after a
 * directive:
 *   'chapter' | 'context' | 'arrow' → fetched from the backend hint lists
 *   'node'   → free text (a node name); no list
 *   'number' → numeric argument; no list
 *   'none'   → boolean flag; takes no argument
 */

/** @type {Directive[]} */
export const DIRECTIVES = [
  {
    token: '\\chapter',
    aliases: ['\\section', '\\in', '\\contents', '\\toc', '\\map'],
    arg: 'name | any',
    valueKind: 'chapter',
    summary: 'Restrict results to chapters whose title contains the name. On its ' +
      'own (no text term) it lists the table of contents instead of a graph.',
    example: '\\chapter "Cooking"',
  },
  {
    token: '\\context',
    aliases: ['\\ctx', '\\as'],
    arg: 'tag[,tag] | any',
    valueKind: 'context',
    summary: 'Restrict results to nodes carrying one of the given context tags. ' +
      'On its own (no text term) it lists the context tags instead of a graph.',
    example: '\\context recipes,desserts',
  },
  {
    token: '\\notes',
    aliases: ['\\browse'],
    arg: 'chapter | any',
    valueKind: 'chapter',
    summary: 'Show the full authored notes (every root node and its arrows) for a ' +
      'chapter or context. This is what the explorer renders as a whole-graph view.',
    example: '\\notes \\context "differentiate under integral"',
  },
  {
    token: '\\from',
    aliases: ['\\path', '\\paths'],
    arg: 'node',
    valueKind: 'node',
    summary: 'Start node for a path search. Pair with \\to. Endpoints resolve to an ' +
      'exact node first, falling back to a looser match.',
    example: '\\from start \\to port',
  },
  {
    token: '\\to',
    aliases: [],
    arg: 'node',
    valueKind: 'node',
    summary: 'Target node for a path search. Use after \\from.',
    example: '\\from start \\to port',
  },
  {
    token: '\\sequence',
    aliases: ['\\seq', '\\story', '\\stories'],
    arg: '',
    valueKind: 'none',
    summary: 'Return ordered sequence / story chains (nodes linked by "then"). ' +
      'Combine with a text term or \\chapter to narrow.',
    example: '\\sequence \\chapter Tutorial',
  },
  {
    token: '\\arrow',
    aliases: ['\\arrows'],
    arg: 'code[,code] | any',
    valueKind: 'arrow',
    summary: 'List or filter relationship (arrow) definitions by short code.',
    example: '\\arrow any',
  },
  {
    token: '\\limit',
    aliases: ['\\depth', '\\range', '\\distance'],
    arg: 'N',
    valueKind: 'number',
    summary: 'Cap the number of results. (All four spellings set the same cap.)',
    example: 'fish \\limit 20',
  },
  {
    token: '\\page',
    aliases: [],
    arg: 'N',
    valueKind: 'number',
    summary: 'Show a single page of a \\notes view. Omit it to aggregate every page.',
    example: '\\notes Cooking \\page 2',
  },
  {
    token: '\\radius',
    aliases: [],
    arg: 'N',
    valueKind: 'number',
    summary: 'How far to expand each match’s neighborhood in the graph explorer ' +
      '(1–3 hops; default 2). \\limit sets how many matches; \\radius sets how far ' +
      'around each one. Larger = more context, denser graph.',
    example: 'eat \\radius 1',
  },
]

/**
 * MATCHING — text-matching semantics. Every row is pinned by a passing test in
 * sstorytime-kv v0.5.0 (bleve_translate_test.go / bleve_index_test.go). Shown
 * in the help panel's "Matching words" section.
 */
export const MATCHING = [
  {
    title: 'Whole words, stemmed',
    body: 'A plain word matches whole words reduced to their stem, ignoring case ' +
      'and accents: "run" matches "running" and "runs". For a substring/begins-with ' +
      'match use a prefix (below).',
  },
  {
    title: 'Multiple words = AND',
    body: 'Words separated by spaces must ALL appear. "recipe fish soup" needs ' +
      'every one of those words. You can also write the & explicitly: a & b.',
  },
  {
    title: 'OR  |   and   NOT  !',
    body: 'a | b matches either; a & !b matches a without b. Precedence follows ' +
      'ts_query: ! binds tightest, then &, then | — so "a & b | c" = (a AND b) OR c.',
  },
  {
    title: 'Exact word  !word!  or  |word|',
    body: 'Wrap a word in ! ! or | | to match it exactly, bypassing the stemmer: ' +
      '"!cat!" matches "cat" (and "Cat") but not "cats".',
  },
  {
    title: 'Begins-with  term:*',
    body: 'A trailing :* matches any word starting with the stem: "flo:*" matches ' +
      '"flower", "floor", "flora".',
  },
  {
    title: 'Phrase  "..."',
    body: 'Quote words to require them adjacent and in order: "fish soup" matches ' +
      '"fish soup recipe" but not "fish curry and soup".',
  },
  {
    title: 'Proximity  <->  and  <N>',
    body: 'a <-> b requires a and b adjacent; a <N> b allows N-1 words between ' +
      'them. Chains work: "strange <-> kind <-> of <-> woman".',
  },
  {
    title: 'Accent folding  (word)',
    body: 'Parentheses fold diacritics: "(fangzi)" matches "fángzi" and "Fangzi". ' +
      'CJK text is matched too: "房子" finds nodes containing 房子.',
  },
  {
    title: 'Precise node endpoints',
    body: '\\from X \\to Y resolves X and Y to exact nodes when they exist, so paths ' +
      'target precise nodes. The shorthand <X|Y> equals \\from X \\to Y, and ' +
      '<X|context|Y> adds a context.',
  },
]

/** COMBINING — the worked example + rule, for the help panel. */
export const COMBINING = {
  rule: 'Directives combine in any order. Text terms search node text; \\chapter ' +
    'and \\context filter that search; \\limit caps it. Adding \\chapter or ' +
    '\\context WITHOUT any text term gives a listing, not a graph — add a text ' +
    'term (or use \\notes) to get nodes.',
  example: 'recipe fish soup \\chapter food \\context food',
  explanation: 'Nodes containing recipe AND fish AND soup, limited to chapters ' +
    'matching "food" and carrying the context tag "food".',
}

// Flat token → directive lookup (canonical + aliases) for analyzeQuery.
const TOKEN_INDEX = (() => {
  const m = new Map()
  for (const d of DIRECTIVES) {
    m.set(d.token.toLowerCase(), d)
    for (const a of d.aliases) m.set(a.toLowerCase(), d)
  }
  return m
})()

// All directive tokens (canonical + alias) for prefix suggestion.
const ALL_TOKENS = (() => {
  const out = []
  for (const d of DIRECTIVES) {
    out.push({ token: d.token, dir: d, canonical: true })
    for (const a of d.aliases) out.push({ token: a, dir: d, canonical: false })
  }
  return out
})()

/**
 * analyzeQuery — decide what to suggest at the caret.
 *
 * @param {string} text  full input value
 * @param {number} [caret] caret position (default end of text)
 * @returns {{kind:'directive'|'value'|'none', prefix:string, replaceFrom:number,
 *            replaceTo:number, valueKind?:string, directive?:Directive}}
 *
 * - 'directive': the word under the caret starts with '\'. prefix includes the
 *   leading backslash; the replace range covers that partial token.
 * - 'value': the caret sits in the argument slot right after a directive whose
 *   value is listable (chapter/context/arrow). prefix is the partial value.
 * - 'none': free text / nothing useful to suggest.
 */
export function analyzeQuery(text, caret) {
  const pos = typeof caret === 'number' ? caret : text.length
  let wordStart = pos
  while (wordStart > 0 && !/\s/.test(text[wordStart - 1])) wordStart--
  const currentWord = text.slice(wordStart, pos)

  // Case 1 — typing a directive.
  if (currentWord.startsWith('\\')) {
    return { kind: 'directive', prefix: currentWord, replaceFrom: wordStart, replaceTo: pos }
  }

  // Case 2 — value position: the token immediately to the left is a directive
  // with a listable value kind.
  const before = text.slice(0, wordStart).trimEnd()
  const prevToken = before.split(/\s+/).pop() || ''
  const dir = TOKEN_INDEX.get(prevToken.toLowerCase())
  if (dir && (dir.valueKind === 'chapter' || dir.valueKind === 'context' || dir.valueKind === 'arrow')) {
    return {
      kind: 'value', prefix: currentWord, replaceFrom: wordStart, replaceTo: pos,
      valueKind: dir.valueKind, directive: dir,
    }
  }

  return { kind: 'none', prefix: currentWord, replaceFrom: wordStart, replaceTo: pos }
}

/**
 * getSuggestions — ranked suggestions for an analysis.
 *
 * @param {ReturnType<typeof analyzeQuery>} analysis
 * @param {{liveValues?: {chapter?:string[], context?:string[], arrow?:string[]}}} [opts]
 * @returns {{label:string, detail:string, section:string, apply:string}[]}
 *
 * `apply` is the full text to substitute for [replaceFrom, replaceTo); it ends
 * with a trailing space so the user can keep typing.
 */
export function getSuggestions(analysis, opts = {}) {
  const live = opts.liveValues || {}
  if (!analysis || analysis.kind === 'none') return []

  if (analysis.kind === 'directive') {
    const p = analysis.prefix.toLowerCase()
    return ALL_TOKENS
      .filter(({ token }) => token.toLowerCase().startsWith(p))
      .sort((a, b) => (b.canonical - a.canonical) || a.token.localeCompare(b.token))
      .map(({ token, dir, canonical }) => ({
        label: token,
        detail: canonical ? dir.summary : `alias of ${dir.token}`,
        section: 'Directives',
        apply: token + ' ',
      }))
  }

  // kind === 'value'
  const kind = analysis.valueKind
  const values = live[kind] || []
  const p = analysis.prefix.toLowerCase()
  const section = kind.charAt(0).toUpperCase() + kind.slice(1) + 's'
  return values
    .filter((v) => v && v.toLowerCase().includes(p))
    .sort((a, b) => {
      const ap = a.toLowerCase().startsWith(p) ? 0 : 1
      const bp = b.toLowerCase().startsWith(p) ? 0 : 1
      return ap - bp || a.localeCompare(b)
    })
    .slice(0, 50)
    .map((v) => ({
      label: v,
      detail: '',
      section,
      apply: (/\s/.test(v) ? `"${v}"` : v) + ' ',
    }))
}
