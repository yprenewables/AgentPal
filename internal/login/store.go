package login

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	Subject     string    `json:"subject"`
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func path(root string) string { return filepath.Join(root, "login.json") }

func Read(root string) (State, error) {
	data, err := os.ReadFile(path(root))
	if os.IsNotExist(err) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func Write(root string, state State) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path(root), data, 0o600)
}

func Clear(root string) error {
	err := os.Remove(path(root))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
