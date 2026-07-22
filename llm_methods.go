package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"texere/internal/llm"
)

const llmMaxTokens = 4096 // per-turn response cap for the chat cell

func newStreamID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "llm-" + hex.EncodeToString(b)
}

// GetLLMConfig returns the global LLM profile configuration.
func (a *App) GetLLMConfig() LLMConfig {
	cfg, _ := loadConfig()
	return cfg.LLM
}

// SaveLLMProfile inserts or updates a profile and persists the config.
func (a *App) SaveLLMProfile(p LLMProfile) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	upsertProfile(&cfg.LLM, p)
	return saveConfig(cfg)
}

// DeleteLLMProfile removes a profile by ID.
func (a *App) DeleteLLMProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	deleteProfile(&cfg.LLM, id)
	return saveConfig(cfg)
}

// SetActiveProfile sets the active default profile.
func (a *App) SetActiveProfile(id string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, ok := findProfile(&cfg.LLM, id); !ok {
		return errors.New("profile not found")
	}
	cfg.LLM.ActiveProfileID = id
	return saveConfig(cfg)
}

// providerFor resolves a profile (by id, or the active default when id=="")
// and builds a llm.Provider plus the resolved model.
func (a *App) providerFor(id string) (llm.Provider, string, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, "", err
	}
	if id == "" {
		id = cfg.LLM.ActiveProfileID
	}
	p, ok := findProfile(&cfg.LLM, id)
	if !ok {
		return nil, "", errors.New("no LLM profile configured")
	}
	key, err := resolveKey(p)
	if err != nil {
		return nil, "", err
	}
	prov, err := llm.New(p.Kind, p.BaseURL, key)
	return prov, p.Model, err
}

// TestLLMProfile runs a tiny streamed turn to validate a profile's
// endpoint/key/model, returning nil on success or the provider error.
func (a *App) TestLLMProfile(id string) error {
	prov, model, err := a.providerFor(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := false
	err = prov.Stream(ctx, llm.Request{
		Model:     model,
		Messages:  []llm.Message{{Role: "user", Content: "ping"}},
		MaxTokens: 16,
	}, func(string) error { got = true; cancel(); return nil })
	// A cancel after the first token surfaces as context.Canceled — treat a
	// stream that produced output as success.
	if got {
		return nil
	}
	return err
}

// StartTurnReq is the payload for StartLLMTurn.
type StartTurnReq struct {
	ProfileID string        `json:"profileId"`
	System    string        `json:"system"`
	Messages  []llm.Message `json:"messages"`
}

// StartLLMTurn streams an assistant response in a goroutine, emitting
// llm:delta / llm:done / llm:error events keyed by the returned streamId.
func (a *App) StartLLMTurn(req StartTurnReq) (string, error) {
	prov, model, err := a.providerFor(req.ProfileID)
	if err != nil {
		return "", err
	}
	streamID := newStreamID()
	ctx, cancel := context.WithCancel(context.Background())

	a.llmMu.Lock()
	a.llmStreams[streamID] = cancel
	a.llmMu.Unlock()

	go func() {
		defer func() {
			a.llmMu.Lock()
			delete(a.llmStreams, streamID)
			a.llmMu.Unlock()
			cancel()
		}()
		err := prov.Stream(ctx, llm.Request{
			Model:     model,
			System:    req.System,
			Messages:  req.Messages,
			MaxTokens: llmMaxTokens,
		}, func(text string) error {
			runtime.EventsEmit(a.ctx, "llm:delta", map[string]string{"streamId": streamID, "text": text})
			return nil
		})
		if err != nil && ctx.Err() == nil {
			runtime.EventsEmit(a.ctx, "llm:error", map[string]string{"streamId": streamID, "message": err.Error()})
			return
		}
		runtime.EventsEmit(a.ctx, "llm:done", map[string]string{"streamId": streamID})
	}()
	return streamID, nil
}

// CancelLLMTurn stops an in-flight turn (the Stop button).
func (a *App) CancelLLMTurn(streamID string) {
	a.llmMu.Lock()
	cancel := a.llmStreams[streamID]
	a.llmMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
