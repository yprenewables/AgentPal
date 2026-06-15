package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"agentpal/internal/codex"
	appconfig "agentpal/internal/config"
	"agentpal/internal/constants"
	"agentpal/internal/peer"
	"agentpal/internal/platform"
	"agentpal/internal/share"
	appsync "agentpal/internal/sync"
	"agentpal/internal/types"
	"agentpal/internal/update"
)

type App struct {
	ctx      context.Context
	server   *share.Server
	serverMu sync.Mutex
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetLocalIPs() ([]string, error) {
	return platform.LocalIPs()
}

func (a *App) InspectCodexDir(path string) (types.CodexInspection, error) {
	return codex.InspectDir(path)
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

func (a *App) StartSharing(req types.ShareRequest) (types.ShareStatus, error) {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	if req.Port == 0 {
		req.Port = constants.DefaultPort
	}
	manifest, baseDir, err := share.BuildManifest(req)
	if err != nil {
		return types.ShareStatus{}, err
	}
	if a.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.server.Stop(ctx)
		cancel()
	}
	server := share.NewServer(baseDir, req.Port, manifest)
	if err := server.Start(); err != nil {
		return types.ShareStatus{}, err
	}
	a.server = server
	localIPs, _ := platform.LocalIPs()
	url := ""
	if len(localIPs) > 0 {
		url = "http://" + localIPs[0] + ":" + strconv.Itoa(req.Port)
	}
	return types.ShareStatus{Running: true, Port: req.Port, LocalIPs: localIPs, URL: url}, nil
}

func (a *App) StopSharing() error {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	if a.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := a.server.Stop(ctx)
	a.server = nil
	return err
}

func (a *App) TestConnection(ip string) (types.PeerStatus, error) {
	return peer.NewClient().Health(ip, constants.DefaultPort)
}

func (a *App) FetchRemoteManifest(ip string) (types.RemoteManifest, error) {
	return peer.NewClient().Manifest(ip, constants.DefaultPort)
}

func (a *App) SyncFromPeer(req types.SyncRequest) (types.SyncResult, error) {
	return appsync.FromPeer(req)
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
