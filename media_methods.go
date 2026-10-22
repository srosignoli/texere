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

// ResolveVideoPlayURL turns a chosen video search result into a URL a <video>
// element can play. It is called once per pick rather than for every result,
// because PeerTube and the Internet Archive each need an extra request.
func (a *App) ResolveVideoPlayURL(providerID string, r mediasearch.Result) (string, error) {
	p, ok := mediasearch.Find(providerID)
	if !ok {
		return "", fmt.Errorf("unknown media provider %q", providerID)
	}
	vr, ok := p.(mediasearch.VideoResolver)
	if !ok {
		return "", fmt.Errorf("provider %q cannot resolve video URLs", providerID)
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchCallTimeout)
	defer cancel()
	return vr.ResolvePlayURL(ctx, r)
}

// ListLanguages returns the selectable pronunciation languages, ordered by
// Lingua Libre recording volume.
func (a *App) ListLanguages() []mediasearch.Language {
	return mediasearch.Languages()
}

// AssetRef is a media file stored in the collection's media/ directory, as
// the notebook's cells refer to it: the asset's name and its MIME type.
type AssetRef struct {
	Asset string `json:"asset"`
	Mime  string `json:"mime"`
}

func toAssetRef(a media.Asset) *AssetRef { return &AssetRef{Asset: a.Name, Mime: a.Mime} }

// FetchMediaURL downloads rawURL into the collection's media/ directory and
// returns its AssetRef. kind is "image" or "audio". Serves both "clicked a
// search result" and "pasted a URL" — they are the same operation.
func (a *App) FetchMediaURL(rawURL, kind string) (*AssetRef, error) {
	dir, err := a.currentDir()
	if err != nil {
		return nil, err
	}
	asset, err := media.FetchRemote(dir, rawURL, kind)
	if err != nil {
		return nil, err
	}
	return toAssetRef(asset), nil
}
