package main

import (
	"context"
	"fmt"
	"time"

	"texere/internal/media"
	"texere/internal/mediasearch"
)

// searchCallTimeout bounds a single SearchMedia call. Wails bindings take no
// context, so the deadline is created here.
const searchCallTimeout = 20 * time.Second

// ProviderInfo describes one search provider to the frontend. Kinds lets the
// panel hide providers that cannot serve the requested kind once audio and
// video arrive.
type ProviderInfo struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Kinds []string `json:"kinds"`
}

// ListMediaProviders returns the available search providers in display order.
func (a *App) ListMediaProviders() []ProviderInfo {
	ps := mediasearch.Providers()
	out := make([]ProviderInfo, 0, len(ps))
	for _, p := range ps {
		out = append(out, ProviderInfo{ID: p.ID(), Name: p.Name(), Kinds: p.Kinds()})
	}
	return out
}

// SearchMedia runs a provider search. It needs no open collection — nothing is
// written until the user picks a result.
func (a *App) SearchMedia(providerID, query, kind string, limit int) ([]mediasearch.Result, error) {
	p, ok := mediasearch.Find(providerID)
	if !ok {
		return nil, fmt.Errorf("unknown media provider %q", providerID)
	}
	if limit <= 0 || limit > 50 {
		limit = 24
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchCallTimeout)
	defer cancel()
	return p.Search(ctx, query, kind, limit)
}

// FetchMediaURL downloads rawURL into the collection's media/ directory and
// returns {asset, mime}. Serves both "clicked a search result" and "pasted a
// URL" — they are the same operation.
func (a *App) FetchMediaURL(rawURL string) (map[string]any, error) {
	a.dirMu.RLock()
	dir := a.collectionDir
	a.dirMu.RUnlock()
	if dir == "" {
		return nil, fmt.Errorf("no collection open")
	}
	asset, err := media.FetchRemote(dir, rawURL)
	if err != nil {
		return nil, err
	}
	return map[string]any{"asset": asset.Name, "mime": asset.Mime}, nil
}
