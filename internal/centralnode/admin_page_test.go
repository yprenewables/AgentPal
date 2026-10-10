package centralnode

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentpal/internal/types"
)

func TestAdminPageIsServedAtAgentPalSubpath(t *testing.T) {
	server := NewServer(types.ServerOptions{CodexDir: t.TempDir(), Port: 28888})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/agentpal/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, marker := range []string{"中心节点管理", "使用 Casdoor 登录", "/v1/admin/status"} {
		if !strings.Contains(recorder.Body.String(), marker) {
			t.Fatalf("page missing %q", marker)
		}
	}
}
