package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type anthropicProvider struct {
	baseURL string
	apiKey  string
}

func (p *anthropicProvider) Stream(ctx context.Context, req Request, onDelta func(string) error) error {
	payload := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   req.Messages, // user/assistant only
		"stream":     true,
	}
	if req.System != "" {
		payload["system"] = req.System
	}
	body, _ := json.Marshal(payload)

	url := strings.TrimRight(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("llm: anthropic %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	return scanSSEData(resp.Body, func(data string) (bool, error) {
		var ev struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return false, nil
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				if err := onDelta(ev.Delta.Text); err != nil {
					return true, err
				}
			}
		case "message_stop":
			return true, nil
		case "error":
			return true, fmt.Errorf("llm: anthropic: %s", ev.Error.Message)
		}
		return false, nil
	})
}
