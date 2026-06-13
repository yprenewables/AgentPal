package share

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agentpal/internal/constants"
	"agentpal/internal/security"
	"agentpal/internal/types"
)

type Server struct {
	baseDir  string
	port     int
	manifest types.RemoteManifest
	server   *http.Server
}

func NewServer(baseDir string, port int, manifest types.RemoteManifest) *Server {
	if port == 0 {
		port = constants.DefaultPort
	}
	server := &Server{baseDir: baseDir, port: port, manifest: manifest}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/manifest", server.handleManifest)
	mux.HandleFunc("/files/", server.handleFile)
	server.server = &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return server
}

func (s *Server) Start() error {
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			println("share server error:", err.Error())
		}
	}()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, types.HealthResponse{App: constants.AppName, Version: constants.AppVersion, Port: s.port})
}

func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.manifest)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/files/")
	switch {
	case rel == "config.toml" && s.manifest.Shared.Config.Enabled:
		s.serveBaseFile(w, r, "config.toml")
	case rel == "auth.json" && s.manifest.Shared.Auth.Enabled:
		s.serveBaseFile(w, r, "auth.json")
	case strings.HasPrefix(rel, "skills/") && s.manifest.Shared.Skills.Enabled:
		s.serveSkillFile(w, r, strings.TrimPrefix(rel, "skills/"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveBaseFile(w http.ResponseWriter, r *http.Request, rel string) {
	path, err := security.SafeJoin(s.baseDir, rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, path)
}

func (s *Server) serveSkillFile(w http.ResponseWriter, r *http.Request, rel string) {
	if err := security.ValidateSkillRelPath(rel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !skillInManifest(s.manifest.Shared.Skills.Files, rel) {
		http.NotFound(w, r)
		return
	}
	path, err := security.SafeJoin(s.baseDir, "skills", rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, path)
}

func skillInManifest(files []types.FileEntry, rel string) bool {
	for _, file := range files {
		if file.Path == rel {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
