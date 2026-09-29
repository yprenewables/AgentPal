package central

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/codex"
	"agentpal/internal/constants"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

//go:embed admin_page.html
var adminPageHTML string

type Server struct {
	options      types.ServerOptions
	httpServer   *http.Server
	BackupDir    string
	adminAuth    *PortalJWTVerifier
	devices      *deviceAuthorizations
	logins       *pendingLogins
	userProfiles *userProfileStore
	settings     *settingsStore
	oauthClient  *http.Client
}

func NewServer(options types.ServerOptions) *Server {
	if options.Port == 0 {
		options.Port = constants.DefaultPort
	}
	s := &Server{options: options, devices: newDeviceAuthorizations()}
	s.logins = newPendingLogins()
	userStore := options.UserStoreDir
	if strings.TrimSpace(userStore) == "" {
		userStore = filepath.Join(os.TempDir(), "agentpal-user-profiles")
	}
	s.userProfiles = newUserProfileStore(userStore)
	s.settings = newSettingsStore(userStore)
	s.oauthClient = &http.Client{Timeout: 15 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/v1/config/manifest", s.manifest)
	mux.HandleFunc("/v1/config/files/skills/", s.skill)
	mux.HandleFunc("/v1/config/mcp/", s.mcp)
	mux.HandleFunc("/v1/auth-backups", s.backups)
	mux.HandleFunc("/v1/auth-backups/", s.backup)
	mux.HandleFunc("/v1/admin/status", s.adminStatus)
	mux.HandleFunc("/agentpal/v1/admin/status", s.adminStatus)
	mux.HandleFunc("/v1/admin/users", s.adminUsers)
	mux.HandleFunc("/v1/admin/me", s.adminMe)
	mux.HandleFunc("/v1/admin/settings", s.adminSettings)
	mux.HandleFunc("/v1/admin/user-profiles", s.adminProfiles)
	mux.HandleFunc("/v1/admin/user-profiles/", s.adminProfiles)
	mux.HandleFunc("/v1/client/device/start", s.deviceStart)
	mux.HandleFunc("/v1/client/device/", s.deviceRoute)
	mux.HandleFunc("/agentpal", s.adminPage)
	mux.HandleFunc("/agentpal/", s.adminPage)
	mux.HandleFunc("/agentpal/authorize", s.authorizePage)
	mux.HandleFunc("/v1/auth/login/start", s.oauthStart)
	mux.HandleFunc("/v1/auth/login/status/", s.oauthStatus)
	mux.HandleFunc("/callback", s.oauthCallback)
	mux.HandleFunc("/v1/auth-profiles", s.authProfiles)
	mux.HandleFunc("/v1/auth-profiles/", s.authProfiles)
	if options.AdminJWTPublic != "" {
		if verifier, err := NewPortalJWTVerifierFromFile(options.AdminJWTPublic, options.AdminJWTIssuer); err == nil {
			s.adminAuth = verifier
		}
	}
	s.httpServer = &http.Server{Addr: ":" + strconv.Itoa(options.Port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

func (s *Server) deviceRoute(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/approve") {
		s.deviceApprove(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/client/device/status/") {
		s.deviceStatus(w, r)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) authorizePage(w http.ResponseWriter, r *http.Request) {
	requestID := r.URL.Query().Get("request")
	if requestID == "" || s.adminAuth == nil {
		http.Error(w, "authorization request is unavailable", http.StatusBadRequest)
		return
	}
	claims, err := s.adminAuth.VerifySessionRequest(r)
	if err != nil {
		http.Error(w, "Portal login required", http.StatusUnauthorized)
		return
	}
	if err := s.approveDevice(requestID, claims.Subject); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><meta name=viewport content=width=device-width><title>AgentPal authorized</title><style>body{font:16px system-ui;max-width:600px;margin:60px auto;padding:20px;color:#172033}</style><h1>AgentPal authorized</h1><p>This device can return to the AgentPal application.</p>")
}

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/agentpal" && r.URL.Path != "/agentpal/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, adminPageHTML)
}

func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	manifest, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, types.AdminStatus{
		Running: true, Port: s.options.Port, CodexDir: s.options.CodexDir,
		Skills: len(manifest.Skills), MCP: len(manifest.MCP), GeneratedAt: manifest.GeneratedAt,
		ScanErrors: manifest.Errors,
	})
}
func (s *Server) Handler() http.Handler          { return s.httpServer.Handler }
func (s *Server) Start() error                   { go func() { _ = s.httpServer.ListenAndServe() }(); return nil }
func (s *Server) Stop(ctx context.Context) error { return s.httpServer.Shutdown(ctx) }
func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"app": constants.AppName, "version": constants.AppVersion, "role": "server", "port": s.options.Port})
}
func (s *Server) manifest(w http.ResponseWriter, _ *http.Request) {
	m, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, m)
}
func (s *Server) skill(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/v1/config/files/skills/")
	if err := security.ValidateSkillRelPath(rel); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m, err := BuildManifest(s.options.CodexDir, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	allowed := false
	for _, skill := range m.Skills {
		for _, file := range skill.Files {
			if file.Path == rel {
				allowed = true
			}
		}
	}
	if !allowed {
		http.NotFound(w, r)
		return
	}
	file, err := security.SafeJoin(s.options.CodexDir, "skills", rel)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	http.ServeFile(w, r, file)
}
func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	name := path.Base(strings.TrimSuffix(r.URL.Path, "/"))
	entries, err := codex.ReadMCP(path.Join(s.options.CodexDir, "config.toml"))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for _, entry := range entries {
		if entry.Name == name {
			w.Header().Set("Content-Type", "application/toml; charset=utf-8")
			_, _ = w.Write(entry.TOML)
			return
		}
	}
	http.NotFound(w, r)
}

func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	root := s.BackupDir
	if root == "" {
		root = filepath.Join(os.TempDir(), "agentpal-auth-backups")
	}
	entries, _ := os.ReadDir(root)
	result := []types.BackupInfo{}
	for _, device := range entries {
		if !device.IsDir() {
			continue
		}
		deviceEntries, _ := os.ReadDir(filepath.Join(root, device.Name()))
		for _, item := range deviceEntries {
			if !item.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, device.Name(), item.Name(), "profile.json"))
			if err != nil {
				continue
			}
			var profile authprofile.AuthProfile
			if json.Unmarshal(data, &profile) != nil {
				continue
			}
			info, _ := item.Info()
			result = append(result, types.BackupInfo{Device: device.Name(), Profile: types.AuthProfile{ID: profile.ID, Name: profile.Name, Type: profile.Type, Provider: profile.Provider, Source: profile.Source}, Size: info.Size(), UpdatedAt: info.ModTime()})
		}
	}
	writeJSON(w, result)
}

func (s *Server) backup(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/auth-backups/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		http.Error(w, "invalid backup path", http.StatusBadRequest)
		return
	}
	root := s.BackupDir
	if root == "" {
		root = filepath.Join(os.TempDir(), "agentpal-auth-backups")
	}
	target := filepath.Join(root, parts[0], parts[1])
	switch r.Method {
	case http.MethodGet:
		var buf bytes.Buffer
		if err := packDir(target, &buf); err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(buf.Bytes())
	case http.MethodPost:
		if r.URL.Query().Get("overwrite") != "true" {
			if _, err := os.Stat(target); err == nil {
				http.Error(w, "backup exists; use overwrite=true", http.StatusConflict)
				return
			}
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20+1))
		if err != nil || len(body) > 5<<20 {
			http.Error(w, "backup archive is too large", http.StatusRequestEntityTooLarge)
			return
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if err := unpackBytes(body, target); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if err := os.RemoveAll(target); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
func packDir(dir string, w io.Writer) error { return authprofile.Pack(dir, w) }
func unpackBytes(data []byte, target string) error {
	temp, err := os.MkdirTemp(filepath.Dir(target), ".agentpal-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	profile, err := authprofile.Unpack(bytes.NewReader(data), int64(len(data)), temp)
	if err != nil {
		return err
	}
	extracted := filepath.Join(temp, profile.ID)
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return os.Rename(extracted, target)
}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
