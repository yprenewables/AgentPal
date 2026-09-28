package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

type MCPEntry struct {
	Name   string `json:"name"`
	TOML   []byte `json:"toml"`
	SHA256 string `json:"sha256"`
}

func ReadMCP(path string) ([]MCPEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var root map[string]any
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	raw, ok := root["mcp_servers"]
	if !ok {
		return nil, nil
	}
	servers, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("mcp_servers must be a table")
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]MCPEntry, 0, len(names))
	for _, name := range names {
		canonical, err := toml.Marshal(servers[name])
		if err != nil {
			return nil, err
		}
		fragment, err := toml.Marshal(map[string]any{"mcp_servers": map[string]any{name: servers[name]}})
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(canonical)
		entries = append(entries, MCPEntry{Name: name, TOML: fragment, SHA256: hex.EncodeToString(hash[:])})
	}
	return entries, nil
}

func UpsertMCP(path string, entries []MCPEntry, deleteNames []string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return err
	}
	replacements := map[string][]byte{}
	for _, entry := range entries {
		if entry.Name == "" {
			return errors.New("MCP name is required")
		}
		replacements["mcp_servers."+entry.Name] = entry.TOML
	}
	deletes := make([]string, 0, len(deleteNames))
	for _, name := range deleteNames {
		deletes = append(deletes, "mcp_servers."+name)
	}
	merged, err := ReplaceTableSubtrees(data, replacements, deletes)
	if err != nil {
		return err
	}
	return atomicWrite(path, merged, 0o600)
}
