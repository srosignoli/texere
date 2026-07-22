// Package llm is a provider-agnostic streaming chat client. It speaks two wire
// formats behind one Provider interface: the OpenAI-compatible
// /chat/completions API (OpenAI, OpenRouter, Google's compat endpoint, local
// servers) and the native Anthropic /v1/messages API.
package llm

import (
	"context"
	"errors"
)

// Message is one turn in a conversation.
type Message struct {
	Role    string `json:"role"` // "system" | "user" | "assistant"
	Content string `json:"content"`
}

// Request is a single streamed completion request.
type Request struct {
	Model     string
	System    string // system prompt (mode framing + assembled context)
	Messages  []Message
	MaxTokens int
}

// Provider streams an assistant response, invoking onDelta for each text chunk.
// It returns when the stream ends or ctx is cancelled; errors carry the
// provider's message where available.
type Provider interface {
	Stream(ctx context.Context, req Request, onDelta func(string) error) error
}

// New builds a Provider for the given kind. baseURL and apiKey are the resolved
// endpoint and secret for the profile.
func New(kind, baseURL, apiKey string) (Provider, error) {
	switch kind {
	case "openai-compatible":
		return &openaiProvider{baseURL: baseURL, apiKey: apiKey}, nil
	case "anthropic":
		return &anthropicProvider{baseURL: baseURL, apiKey: apiKey}, nil
	default:
		return nil, errors.New("llm: unknown provider kind " + kind)
	}
}

type openaiProvider struct{ baseURL, apiKey string }

func (p *openaiProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error {
	return errors.New("not implemented")
}

type anthropicProvider struct{ baseURL, apiKey string }

func (p *anthropicProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error {
	return errors.New("not implemented")
}
