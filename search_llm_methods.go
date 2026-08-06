package main

import (
	"context"
	"strings"
	"time"

	"texere/internal/llm"
)

// translateQueryMaxTokens is small on purpose: the answer is one query line, and
// a low cap makes a runaway model fail fast rather than bill for an essay.
const translateQueryMaxTokens = 256

// translateQueryTimeout bounds the call. internal/llm/client.go leaves the
// streamed body untimed on purpose and relies on the caller's context, so
// without this a provider that stalls after sending headers blocks forever —
// and unlike the chat cell there is no Stop button to escape with. Generous
// enough for a slow local model; short enough that a hang surfaces as an error
// the user can act on.
const translateQueryTimeout = 90 * time.Second

// translateWith is the testable core: one non-streamed turn, accumulated.
// Split from TranslateQuery so a stub provider can drive it without a config
// file or a real endpoint.
func translateWith(prov llm.Provider, model, system, question string) (string, error) {
	return translateWithTimeout(prov, model, system, question, translateQueryTimeout)
}

// translateWithTimeout is translateWith with an injectable deadline, so a test
// can prove the context reaches the provider without waiting out the real
// timeout.
func translateWithTimeout(prov llm.Provider, model, system, question string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var sb strings.Builder
	err := prov.Stream(ctx, llm.Request{
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
