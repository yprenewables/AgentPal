package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"agentpal/internal/authprofile"
	"agentpal/internal/central"
	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/constants"
	"agentpal/internal/types"
	"agentpal/internal/update"
)

type App struct {
	ctx           context.Context
	centralServer *central.Server
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	state, err := appconfig.ReadState()
	if err == nil {
		if codexDir, expandErr := codex.ExpandPath(state.CodexDir); expandErr == nil {
			if auth, readErr := os.ReadFile(filepath.Join(codexDir, "auth.json")); readErr == nil {
				_ = codex.EnsureProviderBearerToken(filepath.Join(codexDir, "config.toml"), auth)
			}
		}
	}
	if err == nil && state.Mode == "server" {
		codexDir, expandErr := codex.ExpandPath(state.CodexDir)
		backupDir, backupErr := codex.ExpandPath(state.BackupDir)
		if expandErr == nil && backupErr == nil {
			a.centralServer = central.NewServer(types.ServerOptions{CodexDir: codexDir, Port: state.Port})
			a.centralServer.BackupDir = backupDir
			_ = a.centralServer.Start()
		}
	}
}

func (a *App) shutdown() {
	if a.centralServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.centralServer.Stop(ctx)
		a.centralServer = nil
	}
}

func (a *App) BrowseFolder() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Select Codex Directory"})
}

func (a *App) GetDefaultCodexDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func (a *App) GetBootstrap() (types.Bootstrap, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.Bootstrap{}, err
	}
	if err := appconfig.Validate(state); err != nil {
		return types.Bootstrap{}, err
	}
	return bootstrapFromState(state, false)
}

func (a *App) SaveSettings(req types.SettingsRequest) (types.Bootstrap, error) {
	previous, err := appconfig.ReadState()
	if err != nil {
		return types.Bootstrap{}, err
	}
	state := appconfig.State{
		Mode:        req.Mode,
		CodexDir:    req.CodexDir,
		CentralHost: req.CentralHost,
		CentralPort: req.CentralPort,
		Port:        req.Port,
		DeviceName:  req.DeviceName,
		BackupDir:   req.BackupDir,
	}
	if err := appconfig.Validate(state); err != nil {
		return types.Bootstrap{}, err
	}
	if err := appconfig.WriteState(state); err != nil {
		return types.Bootstrap{}, err
	}
	restartRequired := previous.Mode != state.Mode || previous.Port != state.Port || previous.CodexDir != state.CodexDir
	return bootstrapFromState(state, restartRequired)
}

func (a *App) RefreshConfiguration() (types.ConfigRefresh, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.ConfigRefresh{}, err
	}
	base := "http://" + state.CentralHost + ":" + strconv.Itoa(state.CentralPort)
	client := central.NewClient(nil, 6*time.Second)
	manifest, err := client.Manifest(base)
	if err != nil {
		return disconnectedRefresh(err.Error()), nil
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.ConfigRefresh{}, err
	}
	diff, err := central.Compare(dir, manifest)
	if err != nil {
		return types.ConfigRefresh{}, err
	}
	return types.ConfigRefresh{Connected: true, Manifest: manifest, Diff: diff}, nil
}

func disconnectedRefresh(message string) types.ConfigRefresh {
	return types.ConfigRefresh{Connected: false, Error: message, Diff: types.ConfigDiff{Items: []types.DiffItem{}}}
}

func (a *App) ApplyConfiguration(req types.ApplyRequest) (types.ApplyResult, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.ApplyResult{}, err
	}
	base := "http://" + state.CentralHost + ":" + strconv.Itoa(state.CentralPort)
	manifest, err := central.NewClient(nil, 60*time.Second).Manifest(base)
	if err != nil {
		return types.ApplyResult{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.ApplyResult{}, err
	}
	return central.Apply(context.Background(), central.NewClient(nil, 60*time.Second), base, dir, manifest, req.Changes)
}

func profileType(p authprofile.AuthProfile) types.AuthProfile {
	return types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: p.Active, APIKeyPreview: p.APIKeyPreview, BaseURL: p.BaseURL, ChatGPTUserID: p.ChatGPTUserID, ChatGPTExpiresAt: p.ChatGPTExpiresAt}
}

func profileTypesFromStore(profiles []authprofile.AuthProfile, current authprofile.CurrentStatus) []types.AuthProfile {
	result := make([]types.AuthProfile, 0, len(profiles))
	for _, profile := range profiles {
		converted := profileType(profile)
		converted.Active = profile.ID == current.CurrentProfileID
		result = append(result, converted)
	}
	return result
}

func (a *App) ListAuthProfiles() ([]types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return nil, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return nil, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return nil, err
	}
	profiles, err := authprofile.NewStore(root).List()
	if err != nil {
		return nil, err
	}
	status, statusErr := authprofile.InspectCurrent(dir, profiles)
	if statusErr != nil {
		return nil, statusErr
	}
	return profileTypesFromStore(profiles, status), nil
}

func (a *App) GetCurrentAuthStatus() (types.CurrentAuthStatus, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.CurrentAuthStatus{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.CurrentAuthStatus{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.CurrentAuthStatus{}, err
	}
	profiles, err := authprofile.NewStore(root).List()
	if err != nil {
		return types.CurrentAuthStatus{}, err
	}
	status, err := authprofile.InspectCurrent(dir, profiles)
	if err != nil {
		return types.CurrentAuthStatus{}, err
	}
	return types.CurrentAuthStatus{APIKeyPreview: status.APIKeyPreview, BaseURL: status.BaseURL, ChatGPTUserID: status.ChatGPTUserID, ChatGPTExpiresAt: status.ChatGPTExpiresAt, APIKeyImported: status.APIKeyImported, ChatGPTImported: status.ChatGPTImported, CurrentProfileID: status.CurrentProfileID}, nil
}

func (a *App) ImportCurrentAPIKey(name string) (types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthProfile{}, err
	}
	profile, err := authprofile.NewStore(root).ImportCurrentAPIKey(dir, name)
	if err != nil {
		return types.AuthProfile{}, err
	}
	return profileType(profile), nil
}
func (a *App) ImportCurrentAuth(name string) (types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	p, err := authprofile.NewStore(root).ImportCurrent(dir, name)
	return profileType(p), err
}
func (a *App) CreateAPIKeyProfile(req types.APIKeyProfileRequest) (types.AuthProfile, error) {
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	p, err := authprofile.NewStore(root).CreateAPIKey(authprofile.APIKeyRequest{Name: req.Name, Provider: req.Provider, BaseURL: req.BaseURL, APIKey: req.APIKey})
	return profileType(p), err
}
func (a *App) ActivateAuthProfile(id string) (types.ProfileSwitchResult, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	r, err := authprofile.NewStore(root).Activate(dir, id)
	return r, err
}

func (a *App) PreviewAuthProfile(id string) (types.ProfileSwitchPreview, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.ProfileSwitchPreview{}, err
	}
	return authprofile.NewStore(root).Preview(dir, id)
}

func (a *App) ActivateAuthProfileWithConflict(id string, allowProviderConflict bool) (types.ProfileSwitchResult, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	dir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.ProfileSwitchResult{}, err
	}
	return authprofile.NewStore(root).Activate(dir, id, allowProviderConflict)
}

func (a *App) DeleteAuthProfile(id string) error {
	state, err := appconfig.ReadState()
	if err != nil {
		return err
	}
	codexDir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return err
	}
	store := authprofile.NewStore(root)
	profiles, err := store.List()
	if err != nil {
		return err
	}
	current, err := authprofile.InspectCurrent(codexDir, profiles)
	if err != nil {
		return err
	}
	return store.Delete(id, current.CurrentProfileID)
}

func centerBase(state appconfig.State) string {
	return "http://" + state.CentralHost + ":" + strconv.Itoa(state.CentralPort)
}

func (a *App) ListRemoteBackups() ([]types.BackupInfo, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return nil, err
	}
	backups, err := central.NewClient(nil, 6*time.Second).ListBackups(centerBase(state))
	if err != nil {
		return nil, err
	}
	return backups, nil
}

func (a *App) UploadAuthProfile(id string, overwrite bool) error {
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return err
	}
	var archive bytes.Buffer
	if err := authprofile.Pack(filepath.Join(root, id), &archive); err != nil {
		return err
	}
	state, err := appconfig.ReadState()
	if err != nil {
		return err
	}
	return central.NewClient(nil, 60*time.Second).UploadBackup(centerBase(state), state.DeviceName, id, bytes.NewReader(archive.Bytes()), overwrite)
}

func (a *App) RestoreAuthProfile(device, id string) (types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	data, err := central.NewClient(nil, 60*time.Second).DownloadBackup(centerBase(state), device, id)
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthProfile{}, err
	}
	profile, err := authprofile.Unpack(bytes.NewReader(data), int64(len(data)), root)
	return profileType(profile), err
}

func (a *App) DeleteRemoteBackup(device, id string) error {
	state, err := appconfig.ReadState()
	if err != nil {
		return err
	}
	return central.NewClient(nil, 6*time.Second).DeleteBackup(centerBase(state), device, id)
}

func bootstrapFromState(state appconfig.State, restartRequired bool) (types.Bootstrap, error) {
	codexDir, err := codex.ExpandPath(state.CodexDir)
	if err != nil {
		return types.Bootstrap{}, err
	}
	backupDir, err := codex.ExpandPath(state.BackupDir)
	if err != nil {
		return types.Bootstrap{}, err
	}
	return types.Bootstrap{
		Mode: state.Mode, CodexDir: codexDir, CentralHost: state.CentralHost,
		CentralPort: state.CentralPort, Port: state.Port, DeviceName: state.DeviceName,
		BackupDir: backupDir, Version: constants.AppVersion, RestartRequired: restartRequired,
	}, nil
}

func (a *App) GetBackupRoot() (string, error) {
	return appconfig.BackupRoot()
}

func (a *App) GetBackupExamplePath() (string, error) {
	root, err := appconfig.BackupRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "YYYYMMDD-HHMMSS"), nil
}

func (a *App) CheckForUpdate() (types.UpdateInfo, error) {
	return update.Check()
}

func (a *App) OpenURL(url string) error {
	if url == "" {
		return errors.New("url is required")
	}
	if a.ctx != nil {
		wailsruntime.BrowserOpenURL(a.ctx, url)
		return nil
	}
	return openURLFallback(url)
}

func (a *App) OpenFolder(path string) error {
	if path == "" {
		return errors.New("folder path is required")
	}
	expanded, err := codex.ExpandPath(path)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", expanded).Start()
	case "windows":
		return exec.Command("explorer", expanded).Start()
	default:
		return exec.Command("xdg-open", expanded).Start()
	}
}

func openURLFallback(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
