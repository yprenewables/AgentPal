package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentpal/internal/constants"
)

type State struct {
	Mode        string `json:"mode"`
	CodexDir    string `json:"codex_dir"`
	CentralHost string `json:"central_host,omitempty"`
	CentralPort int    `json:"central_port,omitempty"`
	Port        int    `json:"port,omitempty"`
	DeviceName  string `json:"device_name"`
	BackupDir   string `json:"backup_dir,omitempty"`
}

var stateDirOverride string

type HistoryEntry struct {
	Time   time.Time `json:"time"`
	Peer   string    `json:"peer"`
	Target string    `json:"target"`
	Items  []string  `json:"items"`
	Backup string    `json:"backup"`
}

func StateDir() (string, error) {
	if stateDirOverride != "" {
		return stateDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.StateDir), nil
}

func ReadState() (State, error) {
	state := defaultState()
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
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	normalizeState(&state)
	return state, nil
}

func WriteState(state State) error {
	normalizeState(&state)
	return writeJSON("config.json", state)
}

func Validate(state State) error {
	if state.Mode != "client" && state.Mode != "server" {
		return errors.New("mode must be client or server")
	}
	if strings.TrimSpace(state.CodexDir) == "" {
		return errors.New("codex_dir is required")
	}
	if strings.TrimSpace(state.DeviceName) == "" {
		return errors.New("device_name is required")
	}
	if state.Mode == "client" {
		if strings.TrimSpace(state.CentralHost) == "" {
			return errors.New("central_host is required in client mode")
		}
		if !validPort(state.CentralPort) {
			return errors.New("central_port must be between 1 and 65535")
		}
		return nil
	}
	if !validPort(state.Port) {
		return errors.New("port must be between 1 and 65535")
	}
	if strings.TrimSpace(state.BackupDir) == "" {
		return errors.New("backup_dir is required in server mode")
	}
	return nil
}

func defaultState() State {
	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "agentpal-device"
	}
	return State{
		Mode:        "client",
		CodexDir:    "~/.codex",
		CentralHost: "127.0.0.1",
		CentralPort: constants.DefaultPort,
		Port:        constants.DefaultPort,
		DeviceName:  hostname,
		BackupDir:   "~/.agentpal/server/auth-backups",
	}
}

func normalizeState(state *State) {
	defaults := defaultState()
	if state.Mode == "" {
		state.Mode = defaults.Mode
	}
	if state.CodexDir == "" {
		state.CodexDir = defaults.CodexDir
	}
	if state.CentralPort == 0 {
		state.CentralPort = defaults.CentralPort
	}
	if state.Port == 0 {
		state.Port = defaults.Port
	}
	if state.DeviceName == "" {
		state.DeviceName = defaults.DeviceName
	}
	if state.BackupDir == "" {
		state.BackupDir = defaults.BackupDir
	}
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
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
