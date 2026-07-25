// Package stt transcribes audio via an OpenAI-compatible /audio/transcriptions
// endpoint (local or cloud). Pure standard library, no CGo.
package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 120 * time.Second}

// Transcribe uploads audio to <baseURL>/audio/transcriptions and returns the
// transcript. apiKey may be "" (no Authorization header) for local servers.
// filename names the multipart file part; its extension signals the format.
func Transcribe(ctx context.Context, baseURL, apiKey, model string, audio []byte, filename string) (string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("model", model); err != nil {
		return "", err
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(audio); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	url := strings.TrimRight(baseURL, "/") + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stt: %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &out); err == nil && out.Text != "" {
		return out.Text, nil
	}
	return strings.TrimSpace(string(data)), nil // plain-text response
}
