package modelcatalog

import "testing"

func TestLookupMatchesDirectAndNormalizedIDs(t *testing.T) {
	c := NewCatalog([]Entry{
		{ID: "anthropic/claude-opus-5.5", Provider: "anthropic", Info: Info{Slug: "claude-opus-5.5", Provider: "anthropic"}},
		{ID: "anthropic/claude-opus-5.5:batch", Provider: "anthropic", Info: Info{Slug: "claude-opus-5.5", Provider: "batch"}},
		{ID: "~deepseek/deepseek-flash-latest", Provider: "deepseek", Info: Info{Slug: "deepseek-flash", Provider: "deepseek"}},
		{ID: "qwen/qwen3.8-27b", Provider: "qwen", Info: Info{Slug: "qwen3.8-27b", Provider: "qwen"}},
		{ID: "openai/gpt-6-sol", Provider: "openai", Info: Info{Slug: "gpt-6-sol", Provider: "openai"}},
	})
	for _, tc := range []struct{ slug, provider string }{
		{"claude-opus-5.5", "anthropic"},
		{"claude-opus-5-5", "anthropic"},
		{"deepseek-flash", "deepseek"},
		{"qwen3.8:27b", "qwen"},
		{"gpt-6-sol", "openai"},
	} {
		info, ok := c.Lookup(tc.slug)
		if !ok || info.Provider != tc.provider {
			t.Fatalf("Lookup(%q) info=%+v ok=%v", tc.slug, info, ok)
		}
	}
	if _, ok := c.Lookup("gpt-image-2"); ok {
		t.Fatal("matched an unknown model")
	}
}

func TestLookupRejectsAmbiguousMatches(t *testing.T) {
	c := NewCatalog([]Entry{
		{ID: "a/shared-name", Provider: "a", Info: Info{Slug: "shared-name", Provider: "a"}},
		{ID: "b/shared-name", Provider: "b", Info: Info{Slug: "shared-name", Provider: "b"}},
	})
	if _, ok := c.Lookup("shared-name"); ok {
		t.Fatal("ambiguous match was accepted")
	}
}
