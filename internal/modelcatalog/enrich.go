package modelcatalog

import "encoding/json"

var effortDescriptions = map[string]string{
	"none":       "No reasoning",
	"minimal":    "Minimal reasoning for the fastest responses",
	"low":        "Fast responses with lighter reasoning",
	"medium":     "Balances speed and reasoning depth for everyday tasks",
	"high":       "Greater reasoning depth for complex problems",
	"xhigh":      "Extra high reasoning depth for complex problems",
	"max":        "Maximum reasoning depth for the hardest problems",
	"ultra":      "Highest reasoning depth with extended compute",
	"persistent": "Reasoning that persists across the turn",
}

type reasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

func encode(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}

// Apply 只写入模型目录提供的参数，保留其余键；不写入任何输出相关字段。
func Apply(model map[string]json.RawMessage, info Info) {
	model["input_modalities"] = encode(info.InputModalities)
	model["context_window"] = encode(info.ContextWindow)
	model["max_context_window"] = encode(info.ContextWindow)
	levels := make([]reasoningLevel, 0, len(info.ReasoningEfforts))
	for _, effort := range info.ReasoningEfforts {
		levels = append(levels, reasoningLevel{Effort: effort, Description: effortDescriptions[effort]})
	}
	model["supported_reasoning_levels"] = encode(levels)
	if info.DefaultEffort != "" {
		model["default_reasoning_level"] = encode(info.DefaultEffort)
	} else {
		delete(model, "default_reasoning_level")
	}
}
