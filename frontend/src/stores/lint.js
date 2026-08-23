// lintResult — the most recent LintCollection() report for the open
// collection.
//
// This holds current state, not a log: every run REPLACES the store
// wholesale, never appends. A defect that has been fixed must disappear the
// next time the linter runs, not linger from a stale pass.
//
// Both the per-file marker (ColumnChapters) and the Problems tab (task 4)
// need "how many defects does file X have", so that lookup is derived once
// here instead of every consumer re-scanning `defects` itself.

import { writable, derived } from 'svelte/store'

import { LintCollection } from '../../wailsjs/go/main/App'

const EMPTY_RESULT = { defects: [], vocabulary: [], filesLinted: 0 }

export const lintResult = writable(EMPTY_RESULT)

/**
 * defectCountByFile — Map<relPath, count>, one entry per file that has at
 * least one CategoryDefect finding. Deliberately built from `defects` only:
 * CategoryVocabulary findings (undeclared arrows) live in `vocabulary` and
 * must never mark a file here — that is the difference between ~7 files
 * flagged and ~39 on the real collection.
 */
export const defectCountByFile = derived(lintResult, ($lintResult) => {
  const counts = new Map()
  for (const defect of $lintResult.defects ?? []) {
    counts.set(defect.file, (counts.get(defect.file) ?? 0) + 1)
  }
  return counts
})

/**
 * runLint — lint the currently open collection and replace the store.
 *
 * Non-fatal by design: a broken linter must never stop a collection from
 * opening, so a failed call is logged and leaves the store empty rather than
 * throwing into the caller's open-collection flow.
 */
export async function runLint() {
  try {
    const result = await LintCollection()
    lintResult.set({
      defects: result?.defects ?? [],
      vocabulary: result?.vocabulary ?? [],
      filesLinted: result?.filesLinted ?? 0,
    })
  } catch (e) {
    console.error('lint: LintCollection failed; Problems will be empty until the next successful run:', e)
    lintResult.set(EMPTY_RESULT)
  }
}
