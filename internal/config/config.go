package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"agentpal/internal/constants"
)

type State struct {
	CodexDir   string `json:"codex_dir"`
	LastPeerIP string `json:"last_peer_ip"`
	Port       int    `json:"port"`
}

type HistoryEntry struct {
	Time   time.Time `json:"time"`
	Peer   string    `json:"peer"`
	Target string    `json:"target"`
	Items  []string  `json:"items"`
	Backup string    `json:"backup"`
}

func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.StateDir), nil
}

func ReadState() (State, error) {
	state := State{CodexDir: "~/.codex", Port: constants.DefaultPort}
	path, err := stateFile("config.json")
	if err != nil {
		return state, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	return state, json.Unmarshal(data, &state)
}

func WriteState(state State) error {
	if state.Port == 0 {
		state.Port = constants.DefaultPort
	}
	return writeJSON("config.json", state)
}

func AppendHistory(entry HistoryEntry) error {
	path, err := stateFile("history.json")
	if err != nil {
		return err
	}
	var history []HistoryEntry
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &history)
	} else if !os.IsNotExist(err) {
		return err
	}
	history = append(history, entry)
	return writeJSON("history.json", history)
}

func BackupRoot() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backups"), nil
}

func stateFile(name string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func writeJSON(name string, value any) error {
	path, err := stateFile(name)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
