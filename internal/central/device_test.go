package central

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentpal/internal/types"
)

func decodeStringField(t *testing.T, data []byte, field string) string {
	t.Helper()
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	return values[field]
}

func TestDeviceAuthorizationCanBeStartedAndApprovedOnce(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888, AllowTestAuth: true})
	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/v1/client/device/start", strings.NewReader(`{"deviceName":"mac-02"}`)))
	if start.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", start.Code, start.Body.String())
	}
	requestID := decodeStringField(t, start.Body.Bytes(), "requestId")

	approve := httptest.NewRecorder()
	approveRequest := httptest.NewRequest(http.MethodPost, "/v1/client/device/"+requestID+"/approve", strings.NewReader(`{"subject":"user-1"}`))
	server.Handler().ServeHTTP(approve, approveRequest)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", approve.Code, approve.Body.String())
	}

	status := httptest.NewRecorder()
	server.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/v1/client/device/status/"+requestID, nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"accessToken"`) {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}

	again := httptest.NewRecorder()
	server.Handler().ServeHTTP(again, httptest.NewRequest(http.MethodGet, "/v1/client/device/status/"+requestID, nil))
	if again.Code != http.StatusGone {
		t.Fatalf("second status=%d body=%s", again.Code, again.Body.String())
	}
}

func TestDeviceAuthorizationExpires(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888})
	server.devices.now = func() time.Time { return time.Unix(100, 0) }
	start := httptest.NewRecorder()
	server.Handler().ServeHTTP(start, httptest.NewRequest(http.MethodPost, "/v1/client/device/start", strings.NewReader(`{"deviceName":"mac-02"}`)))
	requestID := decodeStringField(t, start.Body.Bytes(), "requestId")
	server.devices.now = func() time.Time { return time.Unix(100, 0).Add(deviceRequestTTL + time.Second) }
	status := httptest.NewRecorder()
	server.Handler().ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/v1/client/device/status/"+requestID, nil))
	if status.Code != http.StatusGone {
		t.Fatalf("expired status=%d body=%s", status.Code, status.Body.String())
	}
}
