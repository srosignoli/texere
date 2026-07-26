package mediasearch

// Providers returns the live providers in display order. Both are keyless;
// keyed providers (Pexels, YouTube, Google CSE) arrive in a later slice and
// will be configured through the LLMProfile pattern rather than hardcoded here.
func Providers() []Provider {
	return []Provider{
		Commons{BaseURL: commonsAPI},
		Openverse{BaseURL: openverseAPI},
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
