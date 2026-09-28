package central

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"agentpal/internal/authprofile"
	"agentpal/internal/types"
)

// TestCentralAuthProfileEndToEnd 用 stub Casdoor 串起 start -> callback -> status -> profiles -> 本地同步三态。
func TestCentralAuthProfileEndToEnd(t *testing.T) {
	accessToken := unsignedJWT(t, map[string]any{"sub": "user-1"})
	casdoor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		if payload["code"] != "code-1" || payload["code_verifier"] == "" {
			http.Error(w, "bad exchange", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": accessToken, "expires_in": 3600})
	}))
	defer casdoor.Close()

	userRoot := t.TempDir()
	writeReservedProfile(t, userRoot, "user-1", "yangpu", "yangpu")
	server := NewServer(types.ServerOptions{
		CodexDir: t.TempDir(), Port: 28888, UserStoreDir: userRoot,
		CasdoorURL: casdoor.URL, CasdoorClientID: "cid", OAuthRedirectURL: "http://central.test/callback",
	})
	base := httptest.NewServer(server.Handler())
	defer base.Close()

	client := NewClient(base.Client(), 5*time.Second)

	start, err := client.StartLogin(base.URL)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(start.AuthorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsed.Query().Get("state")

	callback, err := base.Client().Get(base.URL + "/callback?code=code-1&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	defer callback.Body.Close()
	if callback.StatusCode != http.StatusOK {
		t.Fatalf("callback status=%d", callback.StatusCode)
	}

	status, err := client.LoginStatus(base.URL, start.LoginID, start.PollToken)
	if err != nil || status.AccessToken != accessToken {
		t.Fatalf("status=%#v err=%v", status, err)
	}

	list, err := client.CentralAuthProfiles(base.URL, status.AccessToken)
	if err != nil || len(list) != 1 || list[0].Name != "yangpu" {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	detail, err := client.CentralAuthProfile(base.URL, status.AccessToken, "yangpu")
	if err != nil {
		t.Fatal(err)
	}

	store := authprofile.NewStore(t.TempDir())

	// 第一态：本地没有同名 profile -> 新增。
	if _, found, _ := store.FindByReservedName("yangpu"); found {
		t.Fatal("unexpected pre-existing local profile")
	}
	added, err := store.AddCentral(authprofile.CentralProfile{
		Name: detail.Name, Provider: detail.Provider, Type: detail.Type,
		AuthJSON: mustJSON(t, detail.Auth), ConfigTOML: []byte(detail.ConfigAuthToml),
	})
	if err != nil {
		t.Fatal(err)
	}
	if added.ContentHash != list[0].ContentHash {
		t.Fatalf("added hash=%q remote=%q", added.ContentHash, list[0].ContentHash)
	}

	// 第二态：本地与服务端一致 -> 已同步。
	local, found, err := store.FindByReservedName("yangpu")
	if err != nil || !found {
		t.Fatalf("local=%#v found=%v err=%v", local, found, err)
	}
	if local.ContentHash != list[0].ContentHash {
		t.Fatalf("expected synced, local=%q remote=%q", local.ContentHash, list[0].ContentHash)
	}

	// 第三态：本地被改动 -> 需要同步。
	drift := authprofile.CentralProfile{
		Name: detail.Name, Provider: detail.Provider, Type: detail.Type,
		AuthJSON: []byte(`{"OPENAI_API_KEY":"sk-drifted"}`), ConfigTOML: []byte(detail.ConfigAuthToml),
	}
	if _, err := store.OverwriteCentral(local.ID, drift); err != nil {
		t.Fatal(err)
	}
	localHash, err := store.ContentHashFor(local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if localHash == list[0].ContentHash {
		t.Fatal("expected needs_sync after local drift")
	}

	// 覆盖后恢复一致，且不触碰 ~/.codex。
	restored, err := store.OverwriteCentral(local.ID, authprofile.CentralProfile{
		Name: detail.Name, Provider: detail.Provider, Type: detail.Type,
		AuthJSON: mustJSON(t, detail.Auth), ConfigTOML: []byte(detail.ConfigAuthToml),
	})
	if err != nil {
		t.Fatal(err)
	}
	if restored.ContentHash != list[0].ContentHash {
		t.Fatalf("restored hash=%q remote=%q", restored.ContentHash, list[0].ContentHash)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
