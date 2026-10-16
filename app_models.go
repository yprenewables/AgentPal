package main

import (
	"agentpal/internal/authprofile"
	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/modelmanagement"
)

func modelService() (*modelmanagement.Service, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return nil, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return nil, err
	}
	return modelmanagement.New(dir), nil
}
func (a *App) GetModelManagementState() (modelmanagement.State, error) {
	s, err := modelService()
	if err != nil {
		return modelmanagement.State{}, err
	}
	return s.State(), nil
}
func (a *App) DiscoverModels() ([]string, error) {
	s, err := modelService()
	if err != nil {
		return nil, err
	}
	return s.Discover()
}
func (a *App) SaveModels(req modelmanagement.SaveRequest) (modelmanagement.State, error) {
	s, err := modelService()
	if err != nil {
		return modelmanagement.State{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return modelmanagement.State{}, err
	}
	var result modelmanagement.State
	err = authprofile.NewStore(root).WithCurrentModels(s.Dir, func() error {
		var saveErr error
		result, saveErr = s.Save(req)
		return saveErr
	})
	return result, err
}

func (a *App) GetDefaultModelSettings() (modelmanagement.DefaultState, error) {
	s, err := modelService()
	if err != nil {
		return modelmanagement.DefaultState{}, err
	}
	return s.Defaults(), nil
}
func (a *App) SaveDefaultModelSettings(req modelmanagement.DefaultRequest) (modelmanagement.DefaultState, error) {
	s, err := modelService()
	if err != nil {
		return modelmanagement.DefaultState{}, err
	}
	return s.SaveDefaults(req)
}
