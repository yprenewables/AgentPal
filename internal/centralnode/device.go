package centralnode

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const deviceRequestTTL = 10 * time.Minute

type deviceAuthorization struct {
	ID         string
	DeviceName string
	Subject    string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Approved   bool
	Consumed   bool
	Token      string
}

type deviceAuthorizations struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]*deviceAuthorization
}

func newDeviceAuthorizations() *deviceAuthorizations {
	return &deviceAuthorizations{now: time.Now, items: make(map[string]*deviceAuthorization)}
}

func (s *Server) deviceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		DeviceName string `json:"deviceName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.DeviceName) == "" {
		http.Error(w, "deviceName is required", http.StatusBadRequest)
		return
	}
	id, err := randomToken(24)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := s.devices.now()
	s.devices.mu.Lock()
	s.devices.items[id] = &deviceAuthorization{ID: id, DeviceName: request.DeviceName, CreatedAt: now, ExpiresAt: now.Add(deviceRequestTTL)}
	s.devices.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]any{"requestId": id, "authorizeURL": "/agentpal/authorize?request=" + id, "expiresAt": now.Add(deviceRequestTTL)})
}

func (s *Server) deviceApprove(w http.ResponseWriter, r *http.Request) {
	if !s.options.AllowTestAuth {
		http.Error(w, "device approval is handled by the Portal", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/client/device/")
	id = strings.TrimSuffix(id, "/approve")
	var request struct {
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.Subject) == "" {
		http.Error(w, "subject is required", http.StatusBadRequest)
		return
	}
	if err := s.approveDevice(id, request.Subject); err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	writeJSON(w, map[string]string{"status": "approved"})
}

func (s *Server) approveDevice(id, subject string) error {
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	item := s.devices.items[id]
	if item == nil || s.devices.now().After(item.ExpiresAt) || item.Consumed {
		return errors.New("authorization request expired")
	}
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	item.Subject, item.Token, item.Approved = subject, token, true
	return nil
}

func (s *Server) deviceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/client/device/status/")
	s.devices.mu.Lock()
	defer s.devices.mu.Unlock()
	item := s.devices.items[id]
	if item == nil || s.devices.now().After(item.ExpiresAt) {
		http.Error(w, "authorization request expired", http.StatusGone)
		return
	}
	if item.Consumed {
		http.Error(w, "authorization request already consumed", http.StatusGone)
		return
	}
	if !item.Approved {
		writeJSON(w, map[string]string{"status": "pending"})
		return
	}
	item.Consumed = true
	writeJSON(w, map[string]string{"status": "approved", "accessToken": item.Token, "subject": item.Subject})
}

func randomToken(size int) (string, error) {
	if size < 1 {
		return "", errors.New("token size must be positive")
	}
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
