package main

// QueryExample is one worked question→query pair shown to the LLM.
type QueryExample struct {
	Question string `json:"question"`
	Query    string `json:"query"`
}

// queryExamples are the few-shot examples fed to the query-translation prompt.
//
// Every one is verified by TestQueryExamplesReturnResults against the N4L
// corpus in internal/corpustest/corpus. That test is the point: an example
// that returns nothing teaches the model to produce queries that return
// nothing, and a query can be syntactically valid, semantically sensible and
// still empty for pure data reasons. Composing these from the grammar alone
// produced three that looked right and matched no data at all.
//
// They teach SHAPE, not content. The corpus is not the user's collection —
// their real chapter and context names are supplied separately from queryHints.
var queryExamples = []QueryExample{
	{"what do my notes say about pods?", "pod"},
	{"pods in the kubernetes chapter", `pod \chapter Kubernetes`},
	{"everything in the neuroscience chapter", `\notes \chapter neuroscience`},
	{"brain waves in the oscillations context", `waves \context oscillations`},
	{"which context tags mention oscillations", `\context oscillations`},
	{"memory notes in neuroscience", `\notes \chapter neuroscience \context memory`},
	{"how do I get from start to target 1?", `\from start \to target 1`},
	{"what does door lead to?", `\from door`},
	{"the exact phrase thank you", `"thank you"`},
	{"brain but not wave", `brain&!wave`},
	{"terms starting with neuro", `neuro:*`},
	{"the exact word pod, not pods", `!pod!`},
	{"what chapters are there?", `\chapter any`},
	{"what arrows are defined?", `\arrow any`},
	{"things near pod, one hop out", `pod \radius 1`},
	{"tell me the story of Mary", `\story Mary`},
	{"the steps in the astronomy sequence", `\sequence \chapter astronomy`},
	{"what sequences are there?", `\story any`},
	{"notes on differentiating under the integral", `\notes \context "differentiate under integral"`},
}

// ListQueryExamples exposes the verified examples to the frontend, which folds
// them into the system prompt. They live in Go because that is where the corpus
// test can prove them.
func (a *App) ListQueryExamples() []QueryExample {
	out := make([]QueryExample, len(queryExamples))
	copy(out, queryExamples)
	return out
}
