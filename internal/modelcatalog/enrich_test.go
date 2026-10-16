package modelcatalog

import (
	"encoding/json"
	"testing"
)

func decode(t *testing.T, model map[string]json.RawMessage, key string, into any) {
	t.Helper()
	raw, ok := model[key]
	if !ok {
		t.Fatalf("missing key %s", key)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatal(err)
	}
}

func TestApplyWritesOnlyRequestedFields(t *testing.T) {
	model := map[string]json.RawMessage{
		"slug":                    json.RawMessage(`"gpt-6-sol"`),
		"visibility":              json.RawMessage(`"list"`),
		"default_reasoning_level": json.RawMessage(`"low"`),
		"output_modalities":       json.RawMessage(`["text"]`),
	}
	Apply(model, Info{Slug: "gpt-6-sol", Provider: "openai", ContextWindow: 272000,
		InputModalities: []string{"text", "image"}, ReasoningEfforts: []string{"low", "high"}, DefaultEffort: "high"})

	var window, maxWindow int64
	decode(t, model, "context_window", &window)
	decode(t, model, "max_context_window", &maxWindow)
	if window != 272000 || maxWindow != 272000 {
		t.Fatalf("window=%d max=%d", window, maxWindow)
	}
	var modalities []string
	decode(t, model, "input_modalities", &modalities)
	if len(modalities) != 2 || modalities[0] != "text" || modalities[1] != "image" {
		t.Fatalf("modalities=%v", modalities)
	}
	var levels []struct {
		Effort      string `json:"effort"`
		Description string `json:"description"`
	}
	decode(t, model, "supported_reasoning_levels", &levels)
	if len(levels) != 2 || levels[1].Effort != "high" || levels[1].Description == "" {
		t.Fatalf("levels=%+v", levels)
	}
	var effort string
	decode(t, model, "default_reasoning_level", &effort)
	if effort != "high" {
		t.Fatalf("default=%q", effort)
	}
	if string(model["visibility"]) != `"list"` || string(model["slug"]) != `"gpt-6-sol"` || string(model["output_modalities"]) != `["text"]` {
		t.Fatalf("unrelated fields changed: %v", model)
	}
}

func TestApplyClearsDefaultEffortWhenUnknown(t *testing.T) {
	model := map[string]json.RawMessage{"default_reasoning_level": json.RawMessage(`"low"`)}
	Apply(model, Info{ContextWindow: 128000, InputModalities: []string{"text"}, ReasoningEfforts: nil})
	if _, ok := model["default_reasoning_level"]; ok {
		t.Fatal("default_reasoning_level was not removed")
	}
	var levels []any
	decode(t, model, "supported_reasoning_levels", &levels)
	if len(levels) != 0 {
		t.Fatalf("levels=%v", levels)
	}
}
