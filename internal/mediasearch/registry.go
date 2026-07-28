package mediasearch

// Providers returns the live providers in display order. All four are keyless.
// Keyed providers (Forvo, Pexels, YouTube) arrive in a later slice and will be
// configured through the LLMProfile pattern rather than hardcoded here.
func Providers() []Provider {
	return []Provider{
		Commons{BaseURL: commonsAPI},
		Openverse{BaseURL: openverseAPI},
		Pronunciation{BaseURL: commonsAPI},
		CommonsAudio{BaseURL: commonsAPI},
	}
}

// Find returns the provider with the given ID.
func Find(id string) (Provider, bool) {
	for _, p := range Providers() {
		if p.ID() == id {
			return p, true
		}
	}
	return nil, false
}
