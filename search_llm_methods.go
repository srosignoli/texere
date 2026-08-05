package main

import (
	"context"
	"strings"

	"texere/internal/llm"
)

// translateQueryMaxTokens is small on purpose: the answer is one query line, and
// a low cap makes a runaway model fail fast rather than bill for an essay.
const translateQueryMaxTokens = 256

// translateWith is the testable core: one non-streamed turn, accumulated.
// Split from TranslateQuery so a stub provider can drive it without a config
// file or a real endpoint.
func translateWith(prov llm.Provider, model, system, question string) (string, error) {
	var sb strings.Builder
	err := prov.Stream(context.Background(), llm.Request{
		Model:     model,
		System:    system,
		Messages:  []llm.Message{{Role: "user", Content: question}},
		MaxTokens: translateQueryMaxTokens,
	}, func(text string) error {
		sb.WriteString(text)
		return nil
	})
	if err != nil {
		return "", err
	}
	return sb.String(), nil
}

// TranslateQuery turns a natural-language question into a graph query using the
// given profile ("" = the active one). One shot, not streamed: the answer is a
// single line, so the frontend awaits a value instead of reassembling deltas.
//
// The system prompt is passed IN rather than built here, so the query grammar
// keeps exactly one home — frontend/src/lib/query-grammar.js. Rebuilding it in
// Go would be the drift this design exists to avoid.
func (a *App) TranslateQuery(profileID, question, system string) (string, error) {
	prov, model, err := a.providerFor(profileID)
	if err != nil {
		return "", err
	}
	return translateWith(prov, model, system, question)
}

// GetQueryLLMProfile reports which profile translates queries. Empty means the
// active profile, which providerFor already resolves.
func (a *App) GetQueryLLMProfile() string {
	cfg, _ := loadConfig()
	return cfg.QueryLLMProfileID
}

// SetQueryLLMProfile remembers the choice across restarts.
func (a *App) SetQueryLLMProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	cfg.QueryLLMProfileID = id
	return saveConfig(cfg)
}
