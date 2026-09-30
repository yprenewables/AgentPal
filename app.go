package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	"agentpal/internal/login"
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
	return types.AuthProfile{ID: p.ID, Name: p.Name, Type: p.Type, Provider: p.Provider, Source: p.Source, Active: p.Active, APIKeyPreview: p.APIKeyPreview, BaseURL: p.BaseURL, ChatGPTUserID: p.ChatGPTUserID, ChatGPTExpiresAt: p.ChatGPTExpiresAt, ReservedName: p.ReservedName, ContentHash: p.ContentHash}
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
func (a *App) UpdateAPIKeyProfile(id string, req types.APIKeyProfileRequest) (types.AuthProfile, error) {
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	p, err := authprofile.NewStore(root).UpdateAPIKey(id, authprofile.APIKeyRequest{Name: req.Name, Provider: req.Provider, BaseURL: req.BaseURL, APIKey: req.APIKey})
	return profileType(p), err
}

func (a *App) CreateRawProfile(req types.RawProfileRequest) (types.AuthProfile, error) {
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	p, err := authprofile.NewStore(root).CreateRaw(authprofile.RawProfileRequest{Name: req.Name, AuthJSON: req.AuthJSON, ConfigTOML: req.ConfigTOML})
	return profileType(p), err
}

func (a *App) UpdateRawProfile(id string, req types.RawProfileRequest) (types.AuthProfile, error) {
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	p, err := authprofile.NewStore(root).UpdateRaw(id, authprofile.RawProfileRequest{Name: req.Name, AuthJSON: req.AuthJSON, ConfigTOML: req.ConfigTOML})
	return profileType(p), err
}

func (a *App) GetRawProfileContent(id string) (types.RawProfileContent, error) {
	root, _ := codex.ExpandPath("~/.agentpal/auth-profiles")
	auth, config, err := authprofile.NewStore(root).GetRawContent(id)
	return types.RawProfileContent{AuthJSON: auth, ConfigTOML: config}, err
}

func (a *App) GetDefaultProfileTemplate() types.RawProfileContent {
	return types.RawProfileContent{
		AuthJSON:   "{\n  \"OPENAI_API_KEY\": \"sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\"\n}",
		ConfigTOML: "model_provider = \"openai\"\n\n[model_providers.openai]\nname = \"openai\"\nbase_url = \"https://api.openai.com/v1\"\nenv_key = \"OPENAI_API_KEY\"\nwire_api = \"responses\"\n",
	}
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

func findLocalReserved(profiles []authprofile.AuthProfile, name string) (authprofile.AuthProfile, bool) {
	for _, profile := range profiles {
		if profile.ReservedName == name {
			return profile, true
		}
	}
	return authprofile.AuthProfile{}, false
}

func centralProfileFrom(detail types.RemoteAuthProfileDetail) authprofile.CentralProfile {
	authJSON, _ := json.Marshal(detail.Auth)
	return authprofile.CentralProfile{
		Name: detail.Name, Provider: detail.Provider, Type: detail.Type,
		AuthJSON: authJSON, ConfigTOML: []byte(detail.ConfigAuthToml),
	}
}

func (a *App) StartCentralLogin() (types.LoginStart, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.LoginStart{}, err
	}
	result, err := central.NewClient(nil, 15*time.Second).StartLogin(centerBase(state))
	if err != nil {
		return types.LoginStart{}, err
	}
	if a.ctx != nil {
		wailsruntime.BrowserOpenURL(a.ctx, result.AuthorizeURL)
	}
	return result, nil
}

func (a *App) PollCentralLogin(loginID, pollToken string) (types.LoginStatus, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.LoginStatus{}, err
	}
	status, err := central.NewClient(nil, 15*time.Second).LoginStatus(centerBase(state), loginID, pollToken)
	if err != nil {
		return types.LoginStatus{}, err
	}
	if status.Status == "approved" {
		root, err := appconfig.StateDir()
		if err != nil {
			return types.LoginStatus{}, err
		}
		expires := time.Now().Add(24 * time.Hour)
		if status.ExpiresIn > 0 {
			expires = time.Now().Add(time.Duration(status.ExpiresIn) * time.Second)
		}
		if err := login.Write(root, login.State{Subject: status.Subject, Name: status.Name, DisplayName: status.DisplayName, Email: status.Email, AccessToken: status.AccessToken, ExpiresAt: expires}); err != nil {
			return types.LoginStatus{}, err
		}
	}
	return status, nil
}

func (a *App) GetCentralLoginState() (types.CentralLoginState, error) {
	root, err := appconfig.StateDir()
	if err != nil {
		return types.CentralLoginState{}, err
	}
	state, err := login.Read(root)
	if err != nil {
		return types.CentralLoginState{}, err
	}
	if state.AccessToken == "" || time.Now().After(state.ExpiresAt) {
		return types.CentralLoginState{LoggedIn: false}, nil
	}
	return types.CentralLoginState{LoggedIn: true, Subject: state.Subject, Name: state.Name, DisplayName: state.DisplayName, Email: state.Email, ExpiresAt: state.ExpiresAt}, nil
}

func (a *App) LogoutCentralLogin() error {
	root, err := appconfig.StateDir()
	if err != nil {
		return err
	}
	return login.Clear(root)
}

func (a *App) SyncCentralAuthProfiles() (types.AuthSyncReport, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	root, err := appconfig.StateDir()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	session, err := login.Read(root)
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	if session.AccessToken == "" || time.Now().After(session.ExpiresAt) {
		return types.AuthSyncReport{}, errors.New("not logged in")
	}
	client := central.NewClient(nil, 15*time.Second)
	base := centerBase(state)
	remote, err := client.CentralAuthProfiles(base, session.AccessToken)
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	profilesRoot, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	store := authprofile.NewStore(profilesRoot)
	local, err := store.List()
	if err != nil {
		return types.AuthSyncReport{}, err
	}
	report := types.AuthSyncReport{Items: []types.AuthSyncItem{}}
	for _, item := range remote {
		existing, found := findLocalReserved(local, item.Name)
		if !found {
			detail, err := client.CentralAuthProfile(base, session.AccessToken, item.Name)
			if err != nil {
				report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "error", Detail: err.Error()})
				continue
			}
			if _, err := store.AddCentral(centralProfileFrom(detail)); err != nil {
				report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "error", Detail: err.Error()})
				continue
			}
			report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "added"})
			continue
		}
		localHash := existing.ContentHash
		if localHash == "" {
			if computed, err := store.ContentHashFor(existing.ID); err == nil {
				localHash = computed
			}
		}
		if localHash != "" && localHash == item.ContentHash {
			report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "synced"})
			continue
		}
		report.Items = append(report.Items, types.AuthSyncItem{Name: item.Name, Status: "needs_sync"})
	}
	return report, nil
}

func (a *App) ApplyCentralAuthProfile(name string) (types.AuthProfile, error) {
	state, err := appconfig.ReadState()
	if err != nil {
		return types.AuthProfile{}, err
	}
	root, err := appconfig.StateDir()
	if err != nil {
		return types.AuthProfile{}, err
	}
	session, err := login.Read(root)
	if err != nil {
		return types.AuthProfile{}, err
	}
	if session.AccessToken == "" || time.Now().After(session.ExpiresAt) {
		return types.AuthProfile{}, errors.New("not logged in")
	}
	profilesRoot, err := codex.ExpandPath("~/.agentpal/auth-profiles")
	if err != nil {
		return types.AuthProfile{}, err
	}
	store := authprofile.NewStore(profilesRoot)
	local, err := store.List()
	if err != nil {
		return types.AuthProfile{}, err
	}
	existing, found := findLocalReserved(local, name)
	if !found {
		return types.AuthProfile{}, errors.New("no local profile for reserved name: " + name)
	}
	detail, err := central.NewClient(nil, 15*time.Second).CentralAuthProfile(centerBase(state), session.AccessToken, name)
	if err != nil {
		return types.AuthProfile{}, err
	}
	updated, err := store.OverwriteCentral(existing.ID, centralProfileFrom(detail))
	if err != nil {
		return types.AuthProfile{}, err
	}
	return profileType(updated), nil
}
