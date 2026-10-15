package modelmanagement

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

type DefaultState struct {
	Available  bool   `json:"available"`
	Error      string `json:"error,omitempty"`
	ConfigHash string `json:"configHash"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
}
type DefaultRequest struct {
	ConfigHash string `json:"configHash"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
}

func configHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func (s *Service) Defaults() DefaultState {
	_, _, key, err := s.credentials()
	if key == "" {
		return DefaultState{Error: err.Error()}
	}
	data, err := os.ReadFile(filepath.Join(s.Dir, "config.toml"))
	if err != nil {
		return DefaultState{Error: err.Error()}
	}
	var cfg struct {
		Model  string `toml:"model"`
		Effort string `toml:"model_reasoning_effort"`
	}
	if err = toml.Unmarshal(data, &cfg); err != nil {
		return DefaultState{Error: "config.toml is invalid: " + err.Error()}
	}
	return DefaultState{Available: true, ConfigHash: configHash(data), Model: cfg.Model, Effort: cfg.Effort}
}

// Patch exact AST value ranges so comments, multiline strings and unrelated tables stay intact.
func patchDefaults(data []byte, req DefaultRequest) ([]byte, error) {
	return patchConfigValues(data, map[string]string{"model": req.Model, "model_reasoning_effort": req.Effort})
}

func patchConfigValues(data []byte, values map[string]string) ([]byte, error) {
	type edit struct {
		start, end  int
		replacement string
	}
	edits := []edit{}
	seen := map[string]bool{}
	var parser unstable.Parser
	parser.Reset(data)
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind == unstable.Table || node.Kind == unstable.ArrayTable {
			break
		}
		if node.Kind != unstable.KeyValue {
			continue
		}
		keys := node.Key()
		keys.Next()
		key := string(keys.Node().Data)
		if keys.Next() {
			continue
		}
		value, ok := values[key]
		if !ok {
			continue
		}
		seen[key] = true
		span := node.Value().Raw
		start, end := int(span.Offset), int(span.Offset+span.Length)
		if value != "" {
			encoded, _ := toml.Marshal(map[string]string{"v": value})
			replacement := strings.TrimSuffix(strings.TrimPrefix(string(encoded), "v = "), "\n")
			edits = append(edits, edit{start, end, replacement})
		} else {
			// Remove only the assignment, retaining its trailing comment.
			keyStart := node.Key()
			keyStart.Next()
			edits = append(edits, edit{int(keyStart.Node().Raw.Offset), end, ""})
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	result := append([]byte{}, data...)
	for _, e := range edits {
		result = append(append(append([]byte{}, result[:e.start]...), []byte(e.replacement)...), result[e.end:]...)
	}
	var prefix strings.Builder
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !seen[key] && values[key] != "" {
			encoded, _ := toml.Marshal(map[string]string{key: values[key]})
			prefix.Write(encoded)
		}
	}
	result = append([]byte(prefix.String()), result...)
	var check map[string]any
	if err := toml.Unmarshal(result, &check); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) SaveDefaults(req DefaultRequest) (DefaultState, error) {
	saveMu.Lock()
	defer saveMu.Unlock()
	switch req.Effort {
	case "", "none", "minimal", "low", "medium", "high", "xhigh":
	default:
		return DefaultState{}, errors.New("unsupported reasoning effort")
	}
	if strings.ContainsAny(req.Model, "\r\n\x00") {
		return DefaultState{}, errors.New("model must be a single-line identifier")
	}
	state := s.Defaults()
	if !state.Available {
		return DefaultState{}, errors.New(state.Error)
	}
	path := filepath.Join(s.Dir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultState{}, err
	}
	if configHash(data) != req.ConfigHash {
		return DefaultState{}, errors.New("config.toml changed since loading. Reload Models before saving defaults")
	}
	updated, err := patchDefaults(data, req)
	if err != nil {
		return DefaultState{}, err
	}
	f, err := os.CreateTemp(s.Dir, ".model-defaults-")
	if err != nil {
		return DefaultState{}, err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(updated)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return DefaultState{}, err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return DefaultState{}, err
	}
	if configHash(current) != req.ConfigHash {
		return DefaultState{}, errors.New("config.toml changed since loading. Reload Models before saving defaults")
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return DefaultState{}, err
	}
	return s.Defaults(), nil
}
