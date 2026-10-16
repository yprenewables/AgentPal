package modelcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const fixture = `{"data":[
 {"id":"openai/gpt-6-sol","canonical_slug":"openai/gpt-6-sol-20260101","context_length":1050000,
  "architecture":{"input_modalities":["file","image","text"]},
  "reasoning":{"supported_efforts":["max","xhigh","high","medium","low","none"],"default_effort":"medium"}},
 {"id":"z-ai/glm-5.3","context_length":1048576,
  "architecture":{"input_modalities":["text"]},
  "reasoning":{"supported_efforts":["max","high","low","bogus"],"default_effort":"bogus"}},
 {"id":"anthropic/claude-opus-5","context_length":1000000,
  "architecture":{"input_modalities":["text","image","file"]},
  "reasoning":{"supported_efforts":["high"],"default_effort":"high"}}
]}`

func TestClientBuildsInfoWithOverridesAndFiltering(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), URL: server.URL, TTL: time.Hour}
	cat, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	info, ok := cat.Lookup("gpt-6-sol")
	if !ok || info.ContextWindow != ContextWindowOverride || len(info.InputModalities) != 2 || info.InputModalities[0] != "text" || info.InputModalities[1] != "image" || info.DefaultEffort != "medium" {
		t.Fatalf("openai info=%+v ok=%v", info, ok)
	}
	glm, ok := cat.Lookup("glm-5.3")
	if !ok || glm.ContextWindow != 1048576 || glm.DefaultEffort != "" {
		t.Fatalf("glm info=%+v ok=%v", glm, ok)
	}
	if _, ok := cat.Lookup("claude-opus-5"); !ok {
		t.Fatal("anthropic model missing")
	}
	if _, err := c.Catalog(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("expected cached single request, got %d", requests)
	}
}

func TestClientSurfacesRequestFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), URL: server.URL, TTL: time.Hour}
	if _, err := c.Catalog(context.Background()); err == nil {
		t.Fatal("expected error for non-2xx")
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer bad.Close()
	c = &Client{HTTP: bad.Client(), URL: bad.URL, TTL: time.Hour}
	if _, err := c.Catalog(context.Background()); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
